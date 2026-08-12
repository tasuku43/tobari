package companionruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

type protocolFixture struct {
	BindingDigest           string `json:"binding_digest"`
	BrokerBootHex           string `json:"broker_boot_hex"`
	BrokerNonceHex          string `json:"broker_nonce_hex"`
	BrokerToCompanionKeyHex string `json:"broker_to_companion_key_hex"`
	ChallengeHex            string `json:"challenge_hex"`
	ClientProofHex          string `json:"client_proof_hex"`
	CompanionInstanceHex    string `json:"companion_instance_hex"`
	CompanionNonceHex       string `json:"companion_nonce_hex"`
	CompanionToBrokerKeyHex string `json:"companion_to_broker_key_hex"`
	ContextID               string `json:"context_id"`
	DeadlineUnixMS          uint64 `json:"deadline_unix_ms"`
	DriverID                string `json:"driver_id"`
	DriverRevision          string `json:"driver_revision"`
	EpochID                 string `json:"epoch_id"`
	EpochKeyHex             string `json:"epoch_key_hex"`
	GrantRevision           string `json:"grant_revision"`
	ProjectID               string `json:"project_id"`
	Provider                string `json:"provider"`
	ReadyFrameHex           string `json:"ready_frame_hex"`
	RecordID                string `json:"record_id"`
	RequestDigest           string `json:"request_digest"`
	RequestID               string `json:"request_id"`
	RootKeyHex              string `json:"root_key_hex"`
	ServerProofHex          string `json:"server_proof_hex"`
	SessionIDHex            string `json:"session_id_hex"`
	StateGeneration         uint64 `json:"state_generation"`
	TaskDigestHex           string `json:"task_digest_hex"`
	TaskStateHex            string `json:"task_state_hex"`
}

func readProtocolFixture(t *testing.T) protocolFixture {
	t.Helper()
	path := filepath.Join("..", "..", "..", "authbroker", "tests", "fixtures", "companion_protocol_v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture protocolFixture
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func decodeFixtureHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestCompanionProtocolMatchesBrokerInteroperabilityFixture(t *testing.T) {
	t.Parallel()
	fixture := readProtocolFixture(t)
	rootKey := decodeFixtureHex(t, fixture.RootKeyHex)
	epochRaw, err := epochRaw(fixture.EpochID)
	if err != nil {
		t.Fatal(err)
	}
	epochKey, err := deriveEpochKey(rootKey, epochRaw)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(epochKey) != fixture.EpochKeyHex {
		t.Fatalf("epoch key = %x, want %s", epochKey, fixture.EpochKeyHex)
	}
	challenge := decodeFixtureHex(t, fixture.ChallengeHex)
	serverProof := decodeFixtureHex(t, fixture.ServerProofHex)
	instance := decodeFixtureHex(t, fixture.CompanionInstanceHex)
	companionNonce := decodeFixtureHex(t, fixture.CompanionNonceHex)
	bootstrap := &Bootstrap{
		document:   bootstrapDocument{EpochID: fixture.EpochID},
		sessionKey: append([]byte(nil), epochKey...),
	}
	defer bootstrap.Clear()
	source := bytes.NewReader(append(append([]byte(nil), challenge...), serverProof...))
	var destination bytes.Buffer
	channel, err := clientHandshake(
		source, &destination, bootstrap,
		bytes.NewReader(append(append([]byte(nil), instance...), companionNonce...)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(destination.Bytes()); got != fixture.ClientProofHex {
		t.Fatalf("client proof = %s, want %s", got, fixture.ClientProofHex)
	}
	if got := hex.EncodeToString(channel.sessionID[:]); got != fixture.SessionIDHex {
		t.Fatalf("session id = %s, want %s", got, fixture.SessionIDHex)
	}
	brokerKey, companionKey, err := deriveSessionKeys(
		epochKey, epochRaw,
		decodeFixtureHex(t, fixture.BrokerBootHex), decodeFixtureHex(t, fixture.BrokerNonceHex),
		instance, companionNonce, decodeFixtureHex(t, fixture.SessionIDHex),
	)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(brokerKey) != fixture.BrokerToCompanionKeyHex ||
		hex.EncodeToString(companionKey) != fixture.CompanionToBrokerKeyHex {
		t.Fatalf("direction keys broker=%x companion=%x", brokerKey, companionKey)
	}
	frameReader := bytes.NewReader(decodeFixtureHex(t, fixture.ReadyFrameHex))
	frameAEAD, err := newAEAD(brokerKey)
	if err != nil {
		t.Fatal(err)
	}
	frameChannel := &encryptedSession{
		reader: frameReader, receiveAEAD: frameAEAD, receiveDirection: brokerDirection,
	}
	copy(frameChannel.sessionID[:], decodeFixtureHex(t, fixture.SessionIDHex))
	document, payload, err := frameChannel.receive()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(payload)
	if len(payload) != 0 || requireFields(document, "protocol_version", "type", "session_id") != nil ||
		fieldNotEqual(document, "type", "ready") || fieldNotEqual(document, "session_id", fixture.SessionIDHex) {
		t.Fatalf("ready frame = %s payload=%x", mustJSON(document), payload)
	}
	state := decodeFixtureHex(t, fixture.TaskStateHex)
	stateDigest := sha256.Sum256(state)
	request := refreshRequest{
		requestID: fixture.RequestID, deadlineUnixMS: fixture.DeadlineUnixMS,
		contextID: fixture.ContextID, projectID: fixture.ProjectID, provider: fixture.Provider,
		recordID: fixture.RecordID, grantRevision: fixture.GrantRevision,
		stateGeneration: fixture.StateGeneration, driverID: fixture.DriverID,
		driverRevision: fixture.DriverRevision, bindingDigest: fixture.BindingDigest,
		requestDigest: fixture.RequestDigest, stateSHA256: hex.EncodeToString(stateDigest[:]),
	}
	if got := request.computeDigest(); got != fixture.TaskDigestHex {
		t.Fatalf("task digest = %s, want %s", got, fixture.TaskDigestHex)
	}
	if maxCiphertext != 131072 || maxInnerJSON != 8192 || maxInnerPayload != 98304 || maxRefreshStatePayload != 32768 {
		t.Fatalf("protocol bounds frame=%d json=%d result=%d request=%d", maxCiphertext, maxInnerJSON, maxInnerPayload, maxRefreshStatePayload)
	}
}

func TestEncryptedSessionRejectsTamperReplayAndOversizedFrame(t *testing.T) {
	t.Parallel()
	fixture := readProtocolFixture(t)
	key := decodeFixtureHex(t, fixture.BrokerToCompanionKeyHex)
	sessionID := decodeFixtureHex(t, fixture.SessionIDHex)
	frame := decodeFixtureHex(t, fixture.ReadyFrameHex)
	newChannel := func(reader io.Reader) *encryptedSession {
		aead, err := newAEAD(key)
		if err != nil {
			t.Fatal(err)
		}
		channel := &encryptedSession{
			reader: reader, receiveAEAD: aead, receiveDirection: brokerDirection,
		}
		copy(channel.sessionID[:], sessionID)
		return channel
	}
	tampered := append([]byte(nil), frame...)
	tampered[len(tampered)-1] ^= 0x01
	if _, payload, err := newChannel(bytes.NewReader(tampered)).receive(); !errors.Is(err, ErrProtocol) {
		clear(payload)
		t.Fatalf("tampered frame error = %v", err)
	}
	replay := newChannel(bytes.NewReader(frame))
	if _, payload, err := replay.receive(); err != nil {
		clear(payload)
		t.Fatal(err)
	} else {
		clear(payload)
	}
	replay.reader = bytes.NewReader(frame)
	if _, payload, err := replay.receive(); !errors.Is(err, ErrProtocol) {
		clear(payload)
		t.Fatalf("replayed frame error = %v", err)
	}
	oversized := make([]byte, frameHeaderBytes)
	binary.BigEndian.PutUint32(oversized[:4], uint32(maxCiphertext+1))
	if _, payload, err := newChannel(bytes.NewReader(oversized)).receive(); !errors.Is(err, ErrProtocol) {
		clear(payload)
		t.Fatalf("oversized frame error = %v", err)
	}
}

func TestProtocolIntegerConversionsRejectOutOfRangeValues(t *testing.T) {
	t.Parallel()
	if length, ok := boundedUint32Length(maxCiphertext, maxCiphertext); !ok || length != maxCiphertext {
		t.Fatalf("maximum frame length = %d, %t", length, ok)
	}
	for _, test := range []struct {
		length  int
		maximum int
	}{
		{length: -1, maximum: maxCiphertext},
		{length: maxCiphertext + 1, maximum: maxCiphertext},
		{length: 0, maximum: maxCiphertext + 1},
	} {
		if _, ok := boundedUint32Length(test.length, test.maximum); ok {
			t.Fatalf("accepted length=%d maximum=%d", test.length, test.maximum)
		}
	}
	if instant, ok := unixMilliFromUint63(1<<63 - 1); !ok || instant.UnixMilli() != 1<<63-1 {
		t.Fatalf("maximum Unix milliseconds = %d, %t", instant.UnixMilli(), ok)
	}
	if _, ok := unixMilliFromUint63(1 << 63); ok {
		t.Fatal("accepted Unix milliseconds above MaxInt64")
	}
}

func newPeerStalledSession(t *testing.T, connection net.Conn, timeout time.Duration) *encryptedSession {
	t.Helper()
	aead, err := newAEAD(bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	channel := &encryptedSession{
		reader: connection, writer: connection,
		sendAEAD: aead, receiveAEAD: aead,
		sendDirection: companionDirection, receiveDirection: brokerDirection,
		writeTimeout: timeout,
	}
	copy(channel.sessionID[:], bytes.Repeat([]byte{0x72}, 16))
	return channel
}

func TestEncryptedSessionBoundsRefreshAndDrainWritesWhenPeerStopsReading(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]any{
		"refresh_result": {
			"protocol_version": companionProtocolVersion,
			"type":             "refresh_result",
			"request_id":       strings.Repeat("8", 32),
			"task_digest":      strings.Repeat("a", 64),
			"state_generation": uint64(7),
			"ok":               false,
			"error":            "outcome_unknown",
			"payload_length":   0,
		},
		"drain_ack": {
			"protocol_version": companionProtocolVersion,
			"type":             "drain_ack",
			"request_id":       strings.Repeat("9", 32),
		},
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			connection, peer := net.Pipe()
			defer peer.Close()
			channel := newPeerStalledSession(t, connection, 50*time.Millisecond)
			started := time.Now()
			if err := channel.send(document, nil); !errors.Is(err, ErrProtocol) {
				t.Fatalf("blocked send error = %v", err)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("blocked send elapsed = %v", elapsed)
			}
			if !channel.closed.Load() {
				t.Fatal("write failure did not close encrypted session")
			}
			channel.close()
		})
	}
}

func TestEncryptedSessionBoundsUnreadOSPipeWrite(t *testing.T) {
	t.Parallel()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	channel := newPeerStalledSession(t, nil, 50*time.Millisecond)
	channel.reader = reader
	channel.writer = writer
	payload := bytes.Repeat([]byte("x"), maxInnerPayload)
	document := map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "refresh_result",
		"request_id":       strings.Repeat("c", 32),
		"task_digest":      strings.Repeat("d", 64),
		"state_generation": uint64(7),
		"ok":               true,
		"error":            nil,
		"payload_length":   len(payload),
	}
	started := time.Now()
	if err := channel.send(document, payload); !errors.Is(err, ErrProtocol) {
		t.Fatalf("blocked OS pipe send error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("blocked OS pipe send elapsed = %v", elapsed)
	}
	channel.close()
}

func TestEncryptedSessionCloseUnblocksSendWithoutWaitingForSendLock(t *testing.T) {
	t.Parallel()
	connection, peer := net.Pipe()
	defer peer.Close()
	channel := newPeerStalledSession(t, connection, defaultWriteTimeout)
	result := make(chan error, 1)
	go func() {
		result <- channel.send(map[string]any{
			"protocol_version": companionProtocolVersion,
			"type":             "pong",
			"request_id":       strings.Repeat("b", 32),
		}, nil)
	}()
	select {
	case err := <-result:
		t.Fatalf("send returned before close: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	started := time.Now()
	channel.close()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("close elapsed = %v", elapsed)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrProtocol) {
			t.Fatalf("closed send error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not unblock send")
	}
}

func TestSessionContextCancellationClosesPeerStalledWriter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	epoch := bytes.Repeat([]byte{0x73}, 32)
	bootstrap, err := NewBootstrap(
		bytes.NewReader(epoch), bytes.Repeat([]byte{0x74}, 32),
		strings.Repeat("d", 64), 501, 20, t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Clear()
	client, server := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- serveSessionWith(
			ctx, client, client, bootstrap, unusedRefreshDriver{},
			bytes.NewReader(bytes.Repeat([]byte{0x75}, 48)), func() time.Time { return now },
		)
	}()
	broker := brokerHandshake(t, server, bootstrap)
	sessionID := hex.EncodeToString(broker.sessionID[:])
	if err := broker.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "ready",
		"session_id":       sessionID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	// The broker side deliberately does not read ready_ack.
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled stalled session returned nil")
		}
	case <-time.After(time.Second):
		t.Fatal("context cancellation did not close stalled session writer")
	}
	broker.close()
}

func mustJSON(fields map[string]json.RawMessage) string {
	encoded, _ := json.Marshal(fields)
	return string(encoded)
}

type awsCredentialRunner struct {
	expiration time.Time
	calls      int
}

func (r *awsCredentialRunner) Run(_ context.Context, command credentialhost.Command) error {
	r.calls++
	_, err := fmt.Fprintf(
		command.Stdout,
		`{"Version":1,"AccessKeyId":"ASIA1234567890ABCD","SecretAccessKey":"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN","SessionToken":"sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ","Expiration":%q}`,
		r.expiration.UTC().Format(time.RFC3339),
	)
	return err
}

type cancellationRefreshDriver struct {
	started chan struct{}
}

func (d *cancellationRefreshDriver) Refresh(
	ctx context.Context,
	_ credentialhost.State,
) (credentialhost.TemporaryCredentials, credentialhost.State, error) {
	close(d.started)
	<-ctx.Done()
	return credentialhost.TemporaryCredentials{}, credentialhost.State{}, ctx.Err()
}

type stateDocument struct {
	SchemaVersion int    `json:"schema_version"`
	Driver        string `json:"driver"`
	Profile       struct {
		Name               string `json:"name"`
		SSOSession         string `json:"sso_session"`
		StartURL           string `json:"sso_start_url"`
		Region             string `json:"sso_region"`
		AccountID          string `json:"sso_account_id"`
		RoleName           string `json:"sso_role_name"`
		Output             string `json:"output"`
		RegistrationScopes string `json:"sso_registration_scopes"`
	} `json:"profile"`
	Executable struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"aws_executable"`
	Cache []struct {
		Name             string `json:"name"`
		ContentBase64URL string `json:"content_base64url"`
	} `json:"sso_cache"`
}

func testCredentialState(t *testing.T) ([]byte, credentialhost.State) {
	t.Helper()
	directory := t.TempDir()
	executable := filepath.Join(directory, "aws")
	contents := []byte("synthetic reviewed aws executable")
	if err := os.WriteFile(executable, contents, 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	document := stateDocument{SchemaVersion: 1, Driver: credentialhost.SSODriverID}
	document.Profile.Name = "tobari"
	document.Profile.SSOSession = "tobari"
	document.Profile.StartURL = "https://example.awsapps.com/start"
	document.Profile.Region = "us-east-1"
	document.Profile.AccountID = "123456789012"
	document.Profile.RoleName = "Developer"
	document.Profile.Output = "json"
	document.Profile.RegistrationScopes = "sso:account:access"
	document.Executable.Path = executable
	document.Executable.SHA256 = hex.EncodeToString(digest[:])
	document.Cache = make([]struct {
		Name             string `json:"name"`
		ContentBase64URL string `json:"content_base64url"`
	}, 1)
	document.Cache[0].Name = strings.Repeat("a", 40) + ".json"
	document.Cache[0].ContentBase64URL = base64.RawURLEncoding.EncodeToString([]byte(`{"synthetic":true}`))
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	state, err := credentialhost.DecodeState(encoded)
	if err != nil {
		t.Fatalf("DecodeState(%s): %v", encoded, err)
	}
	return encoded, state
}

func brokerHandshake(
	t *testing.T,
	connection net.Conn,
	bootstrap *Bootstrap,
) *encryptedSession {
	t.Helper()
	epoch, err := epochRaw(bootstrap.EpochID())
	if err != nil {
		t.Fatal(err)
	}
	boot := bytes.Repeat([]byte{0x41}, 16)
	brokerNonce := bytes.Repeat([]byte{0x42}, 32)
	challenge := append(append(append(append([]byte(nil), challengeMagic...), epoch...), boot...), brokerNonce...)
	if err := writeFull(connection, challenge); err != nil {
		t.Fatal(err)
	}
	clientProof := make([]byte, clientProofBytes)
	if _, err := io.ReadFull(connection, clientProof); err != nil {
		t.Fatal(err)
	}
	clientHeader := clientProof[:56]
	clientMAC := clientProof[56:]
	expected := computeHMAC(bootstrap.sessionKey, []byte(clientProofDomain), challenge, clientHeader)
	if !bytes.Equal(clientMagic, clientHeader[:8]) || !bytes.Equal(clientMAC, expected) {
		t.Fatal("client handshake proof is invalid")
	}
	instance := clientHeader[8:24]
	clientNonce := clientHeader[24:56]
	sessionID := bytes.Repeat([]byte{0x43}, 16)
	serverHeader := append(append([]byte(nil), serverMagic...), sessionID...)
	serverMAC := computeHMAC(
		bootstrap.sessionKey, []byte(serverProofDomain), challenge, clientHeader, clientMAC, serverHeader,
	)
	if err := writeFull(connection, append(serverHeader, serverMAC...)); err != nil {
		t.Fatal(err)
	}
	brokerKey, companionKey, err := deriveSessionKeys(
		bootstrap.sessionKey, epoch, boot, brokerNonce, instance, clientNonce, sessionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	brokerAEAD, err := newAEAD(brokerKey)
	if err != nil {
		t.Fatal(err)
	}
	companionAEAD, err := newAEAD(companionKey)
	if err != nil {
		t.Fatal(err)
	}
	channel := &encryptedSession{
		reader: connection, writer: connection,
		sendAEAD: brokerAEAD, receiveAEAD: companionAEAD,
		sendDirection: brokerDirection, receiveDirection: companionDirection,
	}
	copy(channel.sessionID[:], sessionID)
	return channel
}

func makeRefreshDocument(request refreshRequest) map[string]any {
	return map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "refresh", "request_id": request.requestID,
		"deadline_unix_ms": request.deadlineUnixMS, "task_digest": request.taskDigest,
		"context_id": request.contextID, "project_id": request.projectID,
		"provider": request.provider, "record_id": request.recordID,
		"grant_revision": request.grantRevision, "state_generation": request.stateGeneration,
		"driver_id": request.driverID, "driver_revision": request.driverRevision,
		"binding_digest": request.bindingDigest, "request_digest": request.requestDigest,
		"state_sha256": request.stateSHA256, "payload_length": len(request.payload),
	}
}

func testRefreshRequest(state []byte, driverRevision string, now time.Time, requestID string) refreshRequest {
	digest := sha256.Sum256(state)
	request := refreshRequest{
		requestID: requestID, deadlineUnixMS: uint64(now.Add(45 * time.Second).UnixMilli()),
		contextID: "context-synthetic", projectID: "project-synthetic", provider: "aws",
		recordID: "record-synthetic", grantRevision: "revision-synthetic", stateGeneration: 7,
		driverID: awsDriverID, driverRevision: driverRevision,
		bindingDigest: strings.Repeat("b", 64), requestDigest: strings.Repeat("c", 64),
		stateSHA256: hex.EncodeToString(digest[:]), payload: append([]byte(nil), state...),
	}
	request.taskDigest = request.computeDigest()
	return request
}

func TestRefreshDeadlineAllowsCrossClockMarginWithinHardMaximum(t *testing.T) {
	t.Parallel()
	hostNow := time.Unix(1_700_000_000, 0).UTC()
	brokerNow := hostNow.Add(5 * time.Second)
	request := testRefreshRequest(
		[]byte(`{"synthetic":true}`), strings.Repeat("a", 64), brokerNow, strings.Repeat("1", 32),
	)
	fields := func(candidate refreshRequest) map[string]json.RawMessage {
		encoded, err := json.Marshal(makeRefreshDocument(candidate))
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}

	parsed, err := parseRefresh(fields(request), request.payload, hostNow)
	if err != nil {
		t.Fatalf("clock-skew margin request was rejected: %v", err)
	}
	clear(parsed.payload)

	request.deadlineUnixMS = uint64(hostNow.Add(maxRefreshDuration + time.Millisecond).UnixMilli())
	request.taskDigest = request.computeDigest()
	if parsed, err = parseRefresh(fields(request), request.payload, hostNow); !errors.Is(err, ErrProtocol) {
		clear(parsed.payload)
		t.Fatalf("deadline above the hard maximum error = %v", err)
	}
}

func receiveMessage(t *testing.T, channel *encryptedSession, expectedType string) (map[string]json.RawMessage, []byte) {
	t.Helper()
	document, payload, err := channel.receive()
	if err != nil {
		t.Fatal(err)
	}
	if fieldNotEqual(document, "type", expectedType) {
		clear(payload)
		t.Fatalf("message = %s, want %s", mustJSON(document), expectedType)
	}
	return document, payload
}

func TestCompanionSessionRefreshesAWSStateAndEnforcesDriverAndLeaseBindings(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	stateBytes, state := testCredentialState(t)
	runner := &awsCredentialRunner{expiration: now.Add(time.Hour)}
	driver := credentialhost.NewDriver(runner)
	epoch := bytes.Repeat([]byte{0x21}, 32)
	bootstrap, err := NewBootstrap(
		bytes.NewReader(epoch), bytes.Repeat([]byte{0x22}, 32),
		strings.Repeat("a", 64), 501, 20, t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Clear()
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientResult := make(chan error, 1)
	go func() {
		clientResult <- serveSessionWith(
			ctx, client, client, bootstrap, driver,
			bytes.NewReader(bytes.Repeat([]byte{0x24}, 48)), func() time.Time { return now },
		)
	}()
	channel := brokerHandshake(t, server, bootstrap)
	sessionID := hex.EncodeToString(channel.sessionID[:])
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion, "type": "ready", "session_id": sessionID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	readyAck, payload := receiveMessage(t, channel, "ready_ack")
	clear(payload)
	if fieldNotEqual(readyAck, "session_id", sessionID) {
		t.Fatalf("ready ack = %s", mustJSON(readyAck))
	}
	pingID := strings.Repeat("1", 32)
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion, "type": "ping", "request_id": pingID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	pong, payload := receiveMessage(t, channel, "pong")
	clear(payload)
	if fieldNotEqual(pong, "request_id", pingID) {
		t.Fatalf("pong = %s", mustJSON(pong))
	}
	request := testRefreshRequest(stateBytes, state.DriverRevision(), now, strings.Repeat("2", 32))
	if err := channel.send(makeRefreshDocument(request), request.payload); err != nil {
		t.Fatal(err)
	}
	accepted, payload := receiveMessage(t, channel, "refresh_accepted")
	clear(payload)
	if fieldNotEqual(accepted, "task_digest", request.taskDigest) {
		t.Fatalf("accepted = %s", mustJSON(accepted))
	}
	result, envelope := receiveMessage(t, channel, "refresh_result")
	if fieldNotEqual(result, "request_id", request.requestID) || string(result["ok"]) != "true" ||
		string(result["error"]) != "null" || len(envelope) == 0 {
		clear(envelope)
		t.Fatalf("result = %s payload=%d", mustJSON(result), len(envelope))
	}
	var decodedEnvelope struct {
		SchemaVersion int    `json:"schema_version"`
		State         string `json:"state_base64url"`
		Credentials   struct {
			Version          int    `json:"version"`
			AccessKeyID      string `json:"access_key_id"`
			SecretAccessKey  string `json:"secret_access_key"`
			SessionToken     string `json:"session_token"`
			ExpirationUnixMS int64  `json:"expiration_unix_ms"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(envelope, &decodedEnvelope); err != nil {
		clear(envelope)
		t.Fatal(err)
	}
	clear(envelope)
	if decodedEnvelope.SchemaVersion != 1 || decodedEnvelope.Credentials.Version != 1 ||
		decodedEnvelope.Credentials.AccessKeyID != "ASIA1234567890ABCD" ||
		decodedEnvelope.Credentials.ExpirationUnixMS != runner.expiration.UnixMilli() {
		t.Fatalf("refresh envelope metadata = %+v", decodedEnvelope)
	}
	updatedState, err := base64.RawURLEncoding.DecodeString(decodedEnvelope.State)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credentialhost.DecodeState(updatedState); err != nil {
		t.Fatalf("updated state is invalid: %v", err)
	}
	clear(updatedState)
	badDriver := testRefreshRequest(stateBytes, strings.Repeat("d", 64), now, strings.Repeat("3", 32))
	if err := channel.send(makeRefreshDocument(badDriver), badDriver.payload); err != nil {
		t.Fatal(err)
	}
	_, payload = receiveMessage(t, channel, "refresh_accepted")
	clear(payload)
	badResult, payload := receiveMessage(t, channel, "refresh_result")
	clear(payload)
	if string(badResult["ok"]) != "false" || fieldNotEqual(badResult, "error", "invalid_state") {
		t.Fatalf("driver mismatch result = %s", mustJSON(badResult))
	}
	badDriverType := testRefreshRequest(stateBytes, state.DriverRevision(), now, strings.Repeat("5", 32))
	badDriverType.driverID = awsConsoleDriverID
	badDriverType.taskDigest = badDriverType.computeDigest()
	if err := channel.send(makeRefreshDocument(badDriverType), badDriverType.payload); err != nil {
		t.Fatal(err)
	}
	_, payload = receiveMessage(t, channel, "refresh_accepted")
	clear(payload)
	badTypeResult, payload := receiveMessage(t, channel, "refresh_result")
	clear(payload)
	if string(badTypeResult["ok"]) != "false" || fieldNotEqual(badTypeResult, "error", "invalid_state") {
		t.Fatalf("driver type mismatch result = %s", mustJSON(badTypeResult))
	}
	runner.expiration = now.Add(20 * time.Second)
	shortLease := testRefreshRequest(stateBytes, state.DriverRevision(), now, strings.Repeat("4", 32))
	if err := channel.send(makeRefreshDocument(shortLease), shortLease.payload); err != nil {
		t.Fatal(err)
	}
	_, payload = receiveMessage(t, channel, "refresh_accepted")
	clear(payload)
	leaseResult, payload := receiveMessage(t, channel, "refresh_result")
	clear(payload)
	if string(leaseResult["ok"]) != "false" || fieldNotEqual(leaseResult, "error", "invalid_state") {
		t.Fatalf("short lease result = %s", mustJSON(leaseResult))
	}
	drainID := strings.Repeat("5", 32)
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion, "type": "drain", "request_id": drainID,
		"deadline_unix_ms": uint64(now.Add(30 * time.Second).UnixMilli()),
	}, nil); err != nil {
		t.Fatal(err)
	}
	drain, payload := receiveMessage(t, channel, "drain_ack")
	clear(payload)
	if fieldNotEqual(drain, "request_id", drainID) {
		t.Fatalf("drain ack = %s", mustJSON(drain))
	}
	cancel()
	_ = server.Close()
	select {
	case err := <-clientResult:
		if err == nil {
			t.Fatal("session returned nil after channel close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not stop")
	}
	if runner.calls != 2 {
		t.Fatalf("AWS refresh calls = %d, want valid and short-lease requests only", runner.calls)
	}
}

func TestCompanionCancellationAcknowledgesReceiptThenReportsStartedOutcomeUnknown(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	stateBytes, state := testCredentialState(t)
	driver := &cancellationRefreshDriver{started: make(chan struct{})}
	epoch := bytes.Repeat([]byte{0x31}, 32)
	bootstrap, err := NewBootstrap(
		bytes.NewReader(epoch), bytes.Repeat([]byte{0x32}, 32),
		strings.Repeat("e", 64), 501, 20, t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Clear()
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientResult := make(chan error, 1)
	go func() {
		clientResult <- serveSessionWith(
			ctx, client, client, bootstrap, driver,
			bytes.NewReader(bytes.Repeat([]byte{0x34}, 48)), func() time.Time { return now },
		)
	}()
	channel := brokerHandshake(t, server, bootstrap)
	sessionID := hex.EncodeToString(channel.sessionID[:])
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion, "type": "ready", "session_id": sessionID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	_, payload := receiveMessage(t, channel, "ready_ack")
	clear(payload)

	request := testRefreshRequest(stateBytes, state.DriverRevision(), now, strings.Repeat("6", 32))
	if err := channel.send(makeRefreshDocument(request), request.payload); err != nil {
		t.Fatal(err)
	}
	_, payload = receiveMessage(t, channel, "refresh_accepted")
	clear(payload)
	select {
	case <-driver.started:
	case <-time.After(time.Second):
		t.Fatal("refresh driver did not start")
	}
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "cancel",
		"request_id":       request.requestID,
		"task_digest":      request.taskDigest,
	}, nil); err != nil {
		t.Fatal(err)
	}
	ack, payload := receiveMessage(t, channel, "cancel_ack")
	clear(payload)
	if fieldNotEqual(ack, "request_id", request.requestID) {
		t.Fatalf("cancel ack = %s", mustJSON(ack))
	}
	result, payload := receiveMessage(t, channel, "refresh_result")
	clear(payload)
	if string(result["ok"]) != "false" || fieldNotEqual(result, "error", "outcome_unknown") {
		t.Fatalf("cancel result = %s", mustJSON(result))
	}

	// A cancel that crossed with the already-sent terminal result is correlated
	// by the bounded tombstone and cannot close or contaminate the next call.
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion,
		"type":             "cancel",
		"request_id":       request.requestID,
		"task_digest":      request.taskDigest,
	}, nil); err != nil {
		t.Fatal(err)
	}
	lateAck, payload := receiveMessage(t, channel, "cancel_ack")
	clear(payload)
	if fieldNotEqual(lateAck, "request_id", request.requestID) {
		t.Fatalf("late cancel ack = %s", mustJSON(lateAck))
	}
	pingID := strings.Repeat("7", 32)
	if err := channel.send(map[string]any{
		"protocol_version": companionProtocolVersion, "type": "ping", "request_id": pingID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	pong, payload := receiveMessage(t, channel, "pong")
	clear(payload)
	if fieldNotEqual(pong, "request_id", pingID) {
		t.Fatalf("pong after late cancel = %s", mustJSON(pong))
	}

	cancel()
	_ = server.Close()
	select {
	case <-clientResult:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not stop")
	}
}

func TestRefreshEnvelopeLeaseBoundsAreClosed(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0).UTC()
	state := []byte(`{"synthetic":true}`)
	valid := func(expiration time.Time) error {
		_, err := encodeRefreshEnvelope(
			state, "ASIA1234567890ABCD", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN",
			"sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ", expiration, now,
		)
		return err
	}
	if err := valid(now.Add(30 * time.Second)); err != nil {
		t.Fatalf("minimum lease rejected: %v", err)
	}
	if err := valid(now.Add(12 * time.Hour)); err != nil {
		t.Fatalf("maximum lease rejected: %v", err)
	}
	if !errors.Is(valid(now.Add(30*time.Second-time.Millisecond)), ErrProtocol) ||
		!errors.Is(valid(now.Add(12*time.Hour+time.Millisecond)), ErrProtocol) {
		t.Fatal("out-of-range lease was accepted")
	}
}
