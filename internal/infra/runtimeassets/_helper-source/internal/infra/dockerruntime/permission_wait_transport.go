package dockerruntime

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type permissionWaitTransportRequest struct {
	SchemaVersion    int    `json:"schema_version"`
	PermissionWaitID string `json:"permission_wait_id"`
}

type permissionWaitTransportFault struct {
	Kind      fault.Kind `json:"kind"`
	Code      string     `json:"code"`
	Message   string     `json:"message"`
	Retryable bool       `json:"retryable"`
}

type permissionWaitTransportResponse struct {
	SchemaVersion int                           `json:"schema_version"`
	OK            bool                          `json:"ok"`
	Result        tobari.PermissionWaitResult   `json:"result,omitempty"`
	Error         *permissionWaitTransportFault `json:"error,omitempty"`
}

func permissionWaitTransportResponseFor(result tobari.PermissionWaitResult, err error) permissionWaitTransportResponse {
	if err == nil {
		return permissionWaitTransportResponse{SchemaVersion: 1, OK: true, Result: result}
	}
	public, ok := fault.PublicCopy(err)
	if !ok {
		public = fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport returned an invalid fault", false)
	}
	return permissionWaitTransportResponse{SchemaVersion: 1, Error: &permissionWaitTransportFault{
		Kind: public.Kind, Code: public.Code, Message: public.Message, Retryable: public.Retryable,
	}}
}

func (r permissionWaitTransportResponse) Validate() error {
	if r.SchemaVersion != 1 || r.OK == (r.Error != nil) {
		return fmt.Errorf("permission wait transport response shape is invalid")
	}
	if r.OK {
		return r.Result.Validate()
	}
	if r.Result != "" || r.Error == nil {
		return fmt.Errorf("permission wait transport fault shape is invalid")
	}
	candidate := fault.New(r.Error.Kind, r.Error.Code, r.Error.Message, r.Error.Retryable)
	if err := candidate.Validate(); err != nil {
		return err
	}
	switch r.Error.Code {
	case "invalid_permission_wait", "invalid_permission_wait_record", "invalid_permission_wait_result", "permission_wait_owner_unavailable", "permission_wait_transport_failed":
		if !r.Error.Retryable {
			return nil
		}
	case "permission_wait_unavailable", "permission_wait_interrupted":
		if r.Error.Retryable {
			return nil
		}
	default:
		return fmt.Errorf("permission wait transport fault is not closed")
	}
	return fmt.Errorf("permission wait transport retryability is invalid")
}

func (a *interactiveWorkspaceAttachment) handlePermissionWaitQuery(connection net.Conn, payload []byte) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request permissionWaitTransportRequest
	if err := decoder.Decode(&request); err != nil || request.SchemaVersion != 1 || tobari.ValidatePermissionWaitID(request.PermissionWaitID) != nil {
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		var unexpected [1]byte
		_, _ = connection.Read(unexpected[:])
		cancel()
	}()
	result, err := a.waits.WaitPermission(ctx, request.PermissionWaitID)
	response := permissionWaitTransportResponseFor(result, err)
	encoded, encodeErr := json.Marshal(response)
	if encodeErr != nil || len(encoded) > tobari.PermissionWaitResponseLimit {
		return
	}
	frame := make([]byte, 4, len(encoded)+4)
	binary.BigEndian.PutUint32(frame, uint32(len(encoded)))
	frame = append(frame, encoded...)
	if err := connection.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}
	_, _ = connection.Write(frame)
}

type permissionWaitOwnerClient struct {
	runtime *Runtime
	session tobari.InteractiveAttachmentSession
}

func newPermissionWaitOwnerClient(runtime *Runtime, session tobari.InteractiveAttachmentSession) (*permissionWaitOwnerClient, error) {
	if runtime == nil || session.Validate() != nil {
		return nil, fmt.Errorf("permission wait owner client is invalid")
	}
	return &permissionWaitOwnerClient{runtime: runtime, session: session}, nil
}

func permissionWaitTransportInterruption(err error) error {
	return fault.Wrap(fault.KindCanceled, "permission_wait_interrupted", "permission wait transport was interrupted", true, err)
}

func permissionWaitOwnerTransportInterruption(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return permissionWaitTransportInterruption(err)
	}
	return permissionWaitOwnerFault()
}

func (c *permissionWaitOwnerClient) WaitPermission(ctx context.Context, id string) (tobari.PermissionWaitResult, error) {
	if err := tobari.ValidatePermissionWaitID(id); err != nil {
		return "", invalidPermissionWaitFault()
	}
	if err := ctx.Err(); err != nil {
		return "", permissionWaitOwnerTransportInterruption(ctx, err)
	}
	connection, err := c.runtime.dialPermissionSession(c.session, 3*time.Second)
	if err != nil {
		return "", permissionWaitOwnerTransportInterruption(ctx, err)
	}
	defer connection.Close()
	cancelDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.SetDeadline(time.Now())
		case <-cancelDone:
		}
	}()
	defer close(cancelDone)
	payload, _ := json.Marshal(permissionWaitTransportRequest{SchemaVersion: 1, PermissionWaitID: id})
	frame := append([]byte("Q"+c.session.IngestionNonce), make([]byte, 4)...)
	binary.BigEndian.PutUint32(frame[len(frame)-4:], uint32(len(payload)))
	frame = append(frame, payload...)
	if _, err := connection.Write(frame); err != nil {
		return "", permissionWaitOwnerTransportInterruption(ctx, err)
	}
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(connection, lengthBytes); err != nil {
		return "", permissionWaitOwnerTransportInterruption(ctx, err)
	}
	length := binary.BigEndian.Uint32(lengthBytes)
	if length == 0 || length > tobari.PermissionWaitResponseLimit {
		return "", fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport response is invalid", false)
	}
	encoded := make([]byte, int(length))
	if _, err := io.ReadFull(connection, encoded); err != nil {
		return "", permissionWaitOwnerTransportInterruption(ctx, err)
	}
	var response permissionWaitTransportResponse
	if err := decodeStrictJSON(encoded, &response); err != nil || response.Validate() != nil {
		return "", fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport response is invalid", false)
	}
	if !response.OK {
		return "", fault.New(response.Error.Kind, response.Error.Code, response.Error.Message, response.Error.Retryable)
	}
	return response.Result, nil
}
