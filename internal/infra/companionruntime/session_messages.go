package companionruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"time"
)

const (
	companionProtocolVersion = 1
	refreshTaskDomain        = "tobari/credential-companion/refresh-task/v1\x00"
	awsDriverID              = "aws_cli_sso"
	maxRefreshDuration       = 60 * time.Second
	maxRefreshStatePayload   = 32 * 1024
	maxDigestComponentBytes  = 128
	minimumCredentialLease   = 30 * time.Second
	maximumCredentialLease   = 12 * time.Hour
)

var (
	hex16Pattern        = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hex32Pattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	safeIDPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,127}$`)
	accessKeyPattern    = regexp.MustCompile(`^[A-Z0-9]{16,128}$`)
	secretKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9/+=_-]{20,128}$`)
	sessionTokenPattern = regexp.MustCompile(`^[A-Za-z0-9/+=_-]+$`)
)

type refreshRequest struct {
	requestID       string
	deadlineUnixMS  uint64
	taskDigest      string
	contextID       string
	projectID       string
	provider        string
	recordID        string
	grantRevision   string
	stateGeneration uint64
	driverID        string
	driverRevision  string
	bindingDigest   string
	requestDigest   string
	stateSHA256     string
	payload         []byte
}

func encodeInner(document map[string]any, payload []byte) ([]byte, error) {
	if document == nil || len(payload) > maxInnerPayload {
		return nil, ErrProtocol
	}
	encoded, err := json.Marshal(document)
	if err != nil || len(encoded) == 0 || len(encoded) > maxInnerJSON || !asciiBytes(encoded) {
		return nil, ErrProtocol
	}
	encodedLength, ok := boundedUint32Length(len(encoded), maxInnerJSON)
	if !ok {
		return nil, ErrProtocol
	}
	result := make([]byte, 4+len(encoded)+len(payload))
	binary.BigEndian.PutUint32(result[:4], encodedLength)
	copy(result[4:], encoded)
	copy(result[4+len(encoded):], payload)
	if len(result)+16 > maxCiphertext {
		clear(result)
		return nil, ErrProtocol
	}
	return result, nil
}

func decodeInner(plaintext []byte) (map[string]json.RawMessage, []byte, error) {
	if len(plaintext) < 5 {
		return nil, nil, ErrProtocol
	}
	jsonLength := int(binary.BigEndian.Uint32(plaintext[:4]))
	if jsonLength < 1 || jsonLength > maxInnerJSON || 4+jsonLength > len(plaintext) {
		return nil, nil, ErrProtocol
	}
	encoded := plaintext[4 : 4+jsonLength]
	if !asciiBytes(encoded) {
		return nil, nil, ErrProtocol
	}
	fields, err := decodeProtocolObject(encoded)
	if err != nil {
		return nil, nil, err
	}
	payload := append([]byte(nil), plaintext[4+jsonLength:]...)
	if len(payload) > maxInnerPayload {
		clear(payload)
		return nil, nil, ErrProtocol
	}
	return fields, payload, nil
}

func decodeProtocolObject(encoded []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrProtocol
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, nameErr := decoder.Token()
		name, ok := nameToken.(string)
		if nameErr != nil || !ok {
			return nil, ErrProtocol
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrProtocol
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, ErrProtocol
		}
		fields[name] = raw
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrProtocol
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrProtocol
	}
	canonical, err := json.Marshal(fields)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return nil, ErrProtocol
	}
	return fields, nil
}

func requireFields(fields map[string]json.RawMessage, expected ...string) error {
	if len(fields) != len(expected) {
		return ErrProtocol
	}
	for _, field := range expected {
		if _, ok := fields[field]; !ok {
			return ErrProtocol
		}
	}
	version, ok := uint63Field(fields, "protocol_version")
	if !ok || version != companionProtocolVersion {
		return ErrProtocol
	}
	return nil
}

func stringField(fields map[string]json.RawMessage, name string) (string, bool) {
	var result string
	if json.Unmarshal(fields[name], &result) != nil || result == "" {
		return "", false
	}
	return result, true
}

func uint63Field(fields map[string]json.RawMessage, name string) (uint64, bool) {
	encoded := string(fields[name])
	if encoded == "" || bytes.ContainsAny([]byte(encoded), ".eE+-") {
		return 0, false
	}
	value, err := strconv.ParseUint(encoded, 10, 63)
	return value, err == nil
}

func parseRefresh(fields map[string]json.RawMessage, payload []byte, now time.Time) (refreshRequest, error) {
	expected := []string{
		"protocol_version", "type", "request_id", "deadline_unix_ms", "task_digest",
		"context_id", "project_id", "provider", "record_id", "grant_revision",
		"state_generation", "driver_id", "driver_revision", "binding_digest",
		"request_digest", "state_sha256", "payload_length",
	}
	if requireFields(fields, expected...) != nil || len(payload) == 0 || len(payload) > maxRefreshStatePayload {
		return refreshRequest{}, ErrProtocol
	}
	messageType, ok := stringField(fields, "type")
	if !ok || messageType != "refresh" {
		return refreshRequest{}, ErrProtocol
	}
	request := refreshRequest{payload: append([]byte(nil), payload...)}
	read := func(name string, destination *string) bool {
		value, valid := stringField(fields, name)
		if valid {
			*destination = value
		}
		return valid
	}
	if !read("request_id", &request.requestID) || !hex16Pattern.MatchString(request.requestID) ||
		!read("task_digest", &request.taskDigest) || !hex32Pattern.MatchString(request.taskDigest) ||
		!read("context_id", &request.contextID) || !safeIDPattern.MatchString(request.contextID) ||
		!read("project_id", &request.projectID) || !safeIDPattern.MatchString(request.projectID) ||
		!read("provider", &request.provider) || request.provider != "aws" ||
		!read("record_id", &request.recordID) || !safeIDPattern.MatchString(request.recordID) ||
		!read("grant_revision", &request.grantRevision) || !safeIDPattern.MatchString(request.grantRevision) ||
		!read("driver_id", &request.driverID) || request.driverID != awsDriverID ||
		!read("driver_revision", &request.driverRevision) || !hex32Pattern.MatchString(request.driverRevision) ||
		!read("binding_digest", &request.bindingDigest) || !hex32Pattern.MatchString(request.bindingDigest) ||
		!read("request_digest", &request.requestDigest) || !hex32Pattern.MatchString(request.requestDigest) ||
		!read("state_sha256", &request.stateSHA256) || !hex32Pattern.MatchString(request.stateSHA256) {
		clear(request.payload)
		return refreshRequest{}, ErrProtocol
	}
	request.deadlineUnixMS, ok = uint63Field(fields, "deadline_unix_ms")
	if _, deadlineOK := unixMilliFromUint63(request.deadlineUnixMS); !ok || !deadlineOK {
		clear(request.payload)
		return refreshRequest{}, ErrProtocol
	}
	request.stateGeneration, ok = uint63Field(fields, "state_generation")
	if !ok {
		clear(request.payload)
		return refreshRequest{}, ErrProtocol
	}
	payloadLength, ok := uint63Field(fields, "payload_length")
	if !ok || payloadLength != uint64(len(payload)) {
		clear(request.payload)
		return refreshRequest{}, ErrProtocol
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != request.stateSHA256 || request.computeDigest() != request.taskDigest {
		clear(request.payload)
		return refreshRequest{}, ErrProtocol
	}
	nowMS := now.UnixMilli()
	if nowMS < 0 || request.deadlineUnixMS <= uint64(nowMS) ||
		request.deadlineUnixMS > uint64(now.Add(maxRefreshDuration).UnixMilli()) {
		clear(request.payload)
		return refreshRequest{}, ErrProtocol
	}
	return request, nil
}

func (r refreshRequest) computeDigest() string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(refreshTaskDomain))
	for _, value := range []string{
		r.requestID, r.contextID, r.projectID, r.provider, r.recordID,
		r.grantRevision, r.driverID, r.driverRevision, r.bindingDigest,
		r.requestDigest, r.stateSHA256,
	} {
		lengthValue, ok := boundedUint32Length(len(value), maxDigestComponentBytes)
		if !ok {
			return ""
		}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], lengthValue)
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	var trailing [16]byte
	binary.BigEndian.PutUint64(trailing[:8], r.stateGeneration)
	binary.BigEndian.PutUint64(trailing[8:], r.deadlineUnixMS)
	_, _ = hash.Write(trailing[:])
	return hex.EncodeToString(hash.Sum(nil))
}

func unixMilliFromUint63(value uint64) (time.Time, bool) {
	if value > 1<<63-1 {
		return time.Time{}, false
	}
	// #nosec G115 -- value is explicitly bounded to MaxInt64 before conversion.
	return time.UnixMilli(int64(value)), true
}

func encodeRefreshEnvelope(
	state []byte,
	accessKey string,
	secretKey string,
	sessionToken string,
	expiration time.Time,
	now time.Time,
) ([]byte, error) {
	if len(state) == 0 || len(state) > maxRefreshStatePayload ||
		!validCredentialLease(now, expiration) ||
		!accessKeyPattern.MatchString(accessKey) ||
		!secretKeyPattern.MatchString(secretKey) ||
		len(sessionToken) < 16 || len(sessionToken) > 16384 ||
		!sessionTokenPattern.MatchString(sessionToken) {
		return nil, ErrProtocol
	}
	document := map[string]any{
		"schema_version":  1,
		"state_base64url": base64.RawURLEncoding.EncodeToString(state),
		"credentials": map[string]any{
			"version":            1,
			"access_key_id":      accessKey,
			"secret_access_key":  secretKey,
			"session_token":      sessionToken,
			"expiration_unix_ms": expiration.UnixMilli(),
		},
	}
	encoded, err := json.Marshal(document)
	if err != nil || len(encoded) == 0 || len(encoded) > maxInnerPayload || !asciiBytes(encoded) {
		clear(encoded)
		return nil, ErrProtocol
	}
	return encoded, nil
}

func validCredentialLease(now, expiration time.Time) bool {
	if now.IsZero() || expiration.IsZero() {
		return false
	}
	nowMS, expirationMS := now.UnixMilli(), expiration.UnixMilli()
	if nowMS < 0 || expirationMS < 0 {
		return false
	}
	return expirationMS >= nowMS+minimumCredentialLease.Milliseconds() &&
		expirationMS <= nowMS+maximumCredentialLease.Milliseconds()
}

func asciiBytes(value []byte) bool {
	for _, current := range value {
		if current > 0x7f {
			return false
		}
	}
	return true
}
