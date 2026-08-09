package companionruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	maxPendingRefreshes    = 32
	maxConcurrentRefreshes = 4
	maxCompletedRefreshes  = 128
)

type activeRefresh struct {
	taskDigest       string
	cancel           context.CancelFunc
	cancelRequested  bool
	cancelAckPending bool
	cancelAckDone    chan struct{}
	providerStarted  bool
}

type companionSessionState struct {
	mu             sync.Mutex
	active         map[string]*activeRefresh
	completed      map[string]string
	completedQueue []string
	slots          chan struct{}
	draining       bool
}

func serveSession(
	ctx context.Context,
	source io.Reader,
	destination io.Writer,
	bootstrap *Bootstrap,
	driver refreshDriver,
) error {
	return serveSessionWith(ctx, source, destination, bootstrap, driver, rand.Reader, time.Now)
}

func serveSessionWith(
	ctx context.Context,
	source io.Reader,
	destination io.Writer,
	bootstrap *Bootstrap,
	driver refreshDriver,
	entropy io.Reader,
	now func() time.Time,
) error {
	if ctx == nil || driver == nil || now == nil {
		return ErrProtocol
	}
	channel, err := clientHandshake(source, destination, bootstrap, entropy)
	if err != nil {
		return err
	}
	stopClose := context.AfterFunc(ctx, channel.close)
	defer func() {
		stopClose()
		channel.close()
	}()
	ready, payload, err := channel.receive()
	if err != nil {
		return err
	}
	defer clear(payload)
	sessionHex := hex.EncodeToString(channel.sessionID[:])
	if len(payload) != 0 || requireFields(ready, "protocol_version", "type", "session_id") != nil ||
		fieldNotEqual(ready, "type", "ready") || fieldNotEqual(ready, "session_id", sessionHex) {
		return ErrProtocol
	}
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "ready_ack",
		"session_id":       sessionHex,
	}, nil); err != nil {
		return err
	}
	state := &companionSessionState{
		active:    make(map[string]*activeRefresh),
		completed: make(map[string]string),
		slots:     make(chan struct{}, maxConcurrentRefreshes),
	}
	defer state.cancelAll()
	for {
		document, messagePayload, err := channel.receive()
		if err != nil {
			clear(messagePayload)
			return err
		}
		messageType, ok := stringField(document, "type")
		if !ok {
			clear(messagePayload)
			return ErrProtocol
		}
		switch messageType {
		case "ping":
			err = handlePing(channel, document, messagePayload)
		case "refresh":
			err = state.handleRefresh(ctx, channel, document, messagePayload, driver, now)
		case "cancel":
			err = state.handleCancel(channel, document, messagePayload)
		case "drain":
			err = state.handleDrain(ctx, channel, document, messagePayload, now)
		default:
			err = ErrProtocol
		}
		clear(messagePayload)
		if err != nil {
			return err
		}
	}
}

func handlePing(channel *encryptedSession, fields map[string]json.RawMessage, payload []byte) error {
	if len(payload) != 0 || requireFields(fields, "protocol_version", "type", "request_id") != nil ||
		fieldNotEqual(fields, "type", "ping") {
		return ErrProtocol
	}
	requestID, ok := stringField(fields, "request_id")
	if !ok || !hex16Pattern.MatchString(requestID) {
		return ErrProtocol
	}
	return channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "pong",
		"request_id":       requestID,
	}, nil)
}

func (s *companionSessionState) handleRefresh(
	parent context.Context,
	channel *encryptedSession,
	fields map[string]json.RawMessage,
	payload []byte,
	driver refreshDriver,
	now func() time.Time,
) error {
	request, err := parseRefresh(fields, payload, now())
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.draining || len(s.active) >= maxPendingRefreshes || s.active[request.requestID] != nil || s.completed[request.requestID] != "" {
		s.mu.Unlock()
		clear(request.payload)
		return ErrProtocol
	}
	deadline, deadlineOK := unixMilliFromUint63(request.deadlineUnixMS)
	if !deadlineOK {
		s.mu.Unlock()
		clear(request.payload)
		return ErrProtocol
	}
	requestContext, cancel := context.WithDeadline(parent, deadline)
	active := &activeRefresh{
		taskDigest:    request.taskDigest,
		cancel:        cancel,
		cancelAckDone: make(chan struct{}),
	}
	s.active[request.requestID] = active
	s.mu.Unlock()
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "refresh_accepted",
		"request_id":       request.requestID,
		"task_digest":      request.taskDigest,
	}, nil); err != nil {
		s.finish(request.requestID)
		clear(request.payload)
		return err
	}
	go s.executeRefresh(requestContext, channel, request, active, driver, now)
	return nil
}

func (s *companionSessionState) executeRefresh(
	ctx context.Context,
	channel *encryptedSession,
	request refreshRequest,
	active *activeRefresh,
	driver refreshDriver,
	now func() time.Time,
) {
	defer active.cancel()
	defer clear(request.payload)
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		s.sendRefreshError(channel, request, active, refreshErrorCode(ctx, ctx.Err()))
		return
	}
	state, err := credentialhost.DecodeState(request.payload)
	if err != nil {
		s.sendRefreshError(channel, request, active, "invalid_state")
		return
	}
	defer state.Clear()
	if state.DriverRevision() != request.driverRevision {
		s.sendRefreshError(channel, request, active, "invalid_state")
		return
	}
	started, cancelRequested := s.beginProvider(ctx, request.requestID, active)
	if !started {
		code := refreshErrorCode(ctx, ctx.Err())
		if cancelRequested && code != "timeout" {
			code = "cancelled"
		}
		s.sendRefreshError(channel, request, active, code)
		return
	}
	credentials, updated, err := driver.Refresh(ctx, state)
	if err != nil {
		code := refreshErrorCode(ctx, err)
		if active.providerStarted && (code == "cancelled" || code == "timeout") {
			code = "outcome_unknown"
		}
		s.sendRefreshError(channel, request, active, code)
		return
	}
	defer credentials.Clear()
	defer updated.Clear()
	encodedState, err := updated.Encode()
	if err != nil {
		s.sendRefreshError(channel, request, active, "invalid_state")
		return
	}
	defer clear(encodedState)
	envelope, err := encodeRefreshEnvelope(
		encodedState,
		credentials.AccessKeyID(), credentials.SecretAccessKey(), credentials.SessionToken(),
		credentials.Expiration(), now(),
	)
	if err != nil {
		s.sendRefreshError(channel, request, active, "invalid_state")
		return
	}
	defer clear(envelope)
	if !s.claimResult(request.requestID, active) {
		return
	}
	_ = channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "refresh_result",
		"request_id":       request.requestID,
		"task_digest":      request.taskDigest,
		"state_generation": request.stateGeneration,
		"ok":               true,
		"error":            nil,
		"payload_length":   len(envelope),
	}, envelope)
}

func (s *companionSessionState) sendRefreshError(
	channel *encryptedSession,
	request refreshRequest,
	active *activeRefresh,
	code string,
) {
	if !s.claimResult(request.requestID, active) {
		return
	}
	_ = channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "refresh_result",
		"request_id":       request.requestID,
		"task_digest":      request.taskDigest,
		"state_generation": request.stateGeneration,
		"ok":               false,
		"error":            code,
		"payload_length":   0,
	}, nil)
}

func refreshErrorCode(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, credentialhost.ErrInvalidState) || errors.Is(err, credentialhost.ErrInvalidCredentials) {
		return "invalid_state"
	}
	if errors.Is(err, credentialhost.ErrInvalidExecutable) {
		return "driver_unavailable"
	}
	return "driver_failed"
}

func (s *companionSessionState) claimResult(requestID string, active *activeRefresh) bool {
	for {
		s.mu.Lock()
		current := s.active[requestID]
		if current != active {
			s.mu.Unlock()
			return false
		}
		if active.cancelAckPending {
			ackDone := active.cancelAckDone
			s.mu.Unlock()
			<-ackDone
			continue
		}
		delete(s.active, requestID)
		s.rememberCompletedLocked(requestID, active.taskDigest)
		s.mu.Unlock()
		return true
	}
}

func (s *companionSessionState) beginProvider(
	ctx context.Context,
	requestID string,
	active *activeRefresh,
) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[requestID] != active {
		return false, false
	}
	if active.cancelRequested || ctx.Err() != nil {
		return false, active.cancelRequested
	}
	active.providerStarted = true
	return true, false
}

func (s *companionSessionState) rememberCompletedLocked(requestID, taskDigest string) {
	if s.completed[requestID] != "" {
		return
	}
	if len(s.completedQueue) >= maxCompletedRefreshes {
		expired := s.completedQueue[0]
		s.completedQueue = s.completedQueue[1:]
		delete(s.completed, expired)
	}
	s.completed[requestID] = taskDigest
	s.completedQueue = append(s.completedQueue, requestID)
}

func (s *companionSessionState) finish(requestID string) {
	s.mu.Lock()
	active := s.active[requestID]
	delete(s.active, requestID)
	s.mu.Unlock()
	if active != nil {
		active.cancel()
	}
}

func (s *companionSessionState) handleCancel(
	channel *encryptedSession,
	fields map[string]json.RawMessage,
	payload []byte,
) error {
	if len(payload) != 0 || requireFields(fields, "protocol_version", "type", "request_id", "task_digest") != nil ||
		fieldNotEqual(fields, "type", "cancel") {
		return ErrProtocol
	}
	requestID, requestOK := stringField(fields, "request_id")
	taskDigest, digestOK := stringField(fields, "task_digest")
	if !requestOK || !digestOK || !hex16Pattern.MatchString(requestID) || !hex32Pattern.MatchString(taskDigest) {
		return ErrProtocol
	}
	s.mu.Lock()
	active := s.active[requestID]
	if active == nil {
		completedDigest := s.completed[requestID]
		s.mu.Unlock()
		if completedDigest != "" && completedDigest == taskDigest {
			return channel.send(map[string]any{
				"protocol_version": companionProtocolVersion,
				"type":             "cancel_ack",
				"request_id":       requestID,
				"task_digest":      taskDigest,
			}, nil)
		}
		return ErrProtocol
	}
	if active.taskDigest != taskDigest || active.cancelRequested {
		s.mu.Unlock()
		return ErrProtocol
	}
	active.cancelRequested = true
	active.cancelAckPending = true
	active.cancel()
	s.mu.Unlock()
	err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "cancel_ack",
		"request_id":       requestID,
		"task_digest":      taskDigest,
	}, nil)
	s.mu.Lock()
	active.cancelAckPending = false
	close(active.cancelAckDone)
	s.mu.Unlock()
	return err
}

func (s *companionSessionState) handleDrain(
	ctx context.Context,
	channel *encryptedSession,
	fields map[string]json.RawMessage,
	payload []byte,
	now func() time.Time,
) error {
	if len(payload) != 0 || requireFields(fields, "protocol_version", "type", "request_id", "deadline_unix_ms") != nil ||
		fieldNotEqual(fields, "type", "drain") {
		return ErrProtocol
	}
	requestID, ok := stringField(fields, "request_id")
	deadlineMS, deadlineOK := uint63Field(fields, "deadline_unix_ms")
	deadline, deadlineTimeOK := unixMilliFromUint63(deadlineMS)
	current := now()
	currentMS := current.UnixMilli()
	if !ok || !deadlineOK || !deadlineTimeOK || !hex16Pattern.MatchString(requestID) || currentMS < 0 ||
		deadlineMS <= uint64(currentMS) || deadlineMS > uint64(current.Add(maxRefreshDuration).UnixMilli()) {
		return ErrProtocol
	}
	s.mu.Lock()
	s.draining = true
	s.mu.Unlock()
	go func() {
		timer := time.NewTicker(10 * time.Millisecond)
		defer timer.Stop()
		for {
			s.mu.Lock()
			remaining := len(s.active)
			if remaining != 0 && !time.Now().Before(deadline) {
				for _, active := range s.active {
					active.cancelRequested = true
					active.cancel()
				}
				// drain_ack closes only the lifecycle drain. Each pending
				// refresh still requires its own terminal result; channel
				// teardown classifies anything unresolved as outcome unknown.
				remaining = 0
			}
			s.mu.Unlock()
			if remaining == 0 {
				_ = channel.send(map[string]any{
					"protocol_version": companionProtocolVersion,
					"type":             "drain_ack",
					"request_id":       requestID,
				}, nil)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	return nil
}

func (s *companionSessionState) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, active := range s.active {
		active.cancelRequested = true
		active.cancel()
	}
}

func fieldNotEqual(fields map[string]json.RawMessage, name, expected string) bool {
	value, ok := stringField(fields, name)
	return !ok || value != expected
}
