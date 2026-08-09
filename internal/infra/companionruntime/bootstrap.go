// Package companionruntime owns the private trusted-host credential companion
// process and its reverse Docker exec transport.
package companionruntime

import (
	"bufio"
	"bytes"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	bootstrapSchemaVersion = 1
	sessionKeyBytes        = 32
	epochBytes             = 32
	maxBootstrapJSONBytes  = 8 * 1024
	epochPrefix            = "companion-e1_"
	epochKeyInfo           = "tobari/credential-companion/epoch-key/v1"
	epochSaltDomain        = "tobari/credential-companion/salt/v1\x00"
)

var (
	ErrInvalidBootstrap = errors.New("credential companion bootstrap is invalid")
	ErrUnavailable      = errors.New("credential companion is unavailable")

	containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type bootstrapDocument struct {
	SchemaVersion    int    `json:"schema_version"`
	EpochID          string `json:"epoch_id"`
	ContainerID      string `json:"container_id"`
	UID              int    `json:"uid"`
	GID              int    `json:"gid"`
	StateDirectory   string `json:"state_directory"`
	SessionKeyLength int    `json:"session_key_length"`
}

// Bootstrap is the single-use process bootstrap. Its session key is never
// exposed through formatting, argv, environment, or a file.
type Bootstrap struct {
	document   bootstrapDocument
	sessionKey []byte
}

// NewBootstrap derives one fresh epoch key from the installation root key and
// binds it to the exact broker container selected by the lifecycle operation.
func NewBootstrap(
	entropy io.Reader,
	rootKey []byte,
	containerID string,
	uid int,
	gid int,
	stateDirectory string,
) (*Bootstrap, error) {
	if entropy == nil || len(rootKey) != sessionKeyBytes {
		return nil, ErrInvalidBootstrap
	}
	epochRaw := make([]byte, epochBytes)
	if _, err := io.ReadFull(entropy, epochRaw); err != nil {
		clear(epochRaw)
		return nil, fmt.Errorf("%w: epoch entropy", ErrUnavailable)
	}
	defer clear(epochRaw)
	epochID := epochPrefix + base64.RawURLEncoding.EncodeToString(epochRaw)
	key, err := deriveEpochKey(rootKey, epochRaw)
	if err != nil {
		return nil, err
	}
	bootstrap := &Bootstrap{
		document: bootstrapDocument{
			SchemaVersion: bootstrapSchemaVersion,
			EpochID:       epochID, ContainerID: containerID,
			UID: uid, GID: gid, StateDirectory: stateDirectory,
			SessionKeyLength: sessionKeyBytes,
		},
		sessionKey: key,
	}
	if err := validateBootstrapDocument(bootstrap.document); err != nil {
		bootstrap.Clear()
		return nil, err
	}
	return bootstrap, nil
}

func deriveEpochKey(rootKey, epochRaw []byte) ([]byte, error) {
	if len(rootKey) != sessionKeyBytes || len(epochRaw) != epochBytes {
		return nil, ErrInvalidBootstrap
	}
	saltInput := make([]byte, 0, len(epochSaltDomain)+len(epochRaw))
	saltInput = append(saltInput, epochSaltDomain...)
	saltInput = append(saltInput, epochRaw...)
	salt := sha256.Sum256(saltInput)
	clear(saltInput)
	key, err := hkdf.Key(sha256.New, rootKey, salt[:], epochKeyInfo, sessionKeyBytes)
	clear(salt[:])
	if err != nil {
		return nil, fmt.Errorf("%w: derive epoch key", ErrUnavailable)
	}
	return key, nil
}

// EpochID returns the non-secret epoch identifier used for broker preparation
// and readiness correlation.
func (b *Bootstrap) EpochID() string {
	if b == nil {
		return ""
	}
	return b.document.EpochID
}

func (b *Bootstrap) ContainerID() string {
	if b == nil {
		return ""
	}
	return b.document.ContainerID
}

func (b *Bootstrap) StateDirectory() string {
	if b == nil {
		return ""
	}
	return b.document.StateDirectory
}

func (b *Bootstrap) String() string   { return "companionruntime.Bootstrap{redacted}" }
func (b *Bootstrap) GoString() string { return "companionruntime.Bootstrap{redacted}" }

// Encode returns the exact stdin bytes for the private child. The caller must
// clear the returned buffer after it has been synchronously delivered.
func (b *Bootstrap) Encode() ([]byte, error) {
	if b == nil || len(b.sessionKey) != sessionKeyBytes || validateBootstrapDocument(b.document) != nil {
		return nil, ErrInvalidBootstrap
	}
	encoded, err := json.Marshal(b.document)
	if err != nil || len(encoded) == 0 || len(encoded) > maxBootstrapJSONBytes {
		return nil, ErrInvalidBootstrap
	}
	result := make([]byte, 0, len(encoded)+1+len(b.sessionKey))
	result = append(result, encoded...)
	result = append(result, '\n')
	result = append(result, b.sessionKey...)
	return result, nil
}

// Clear releases the caller-owned copy of the derived session key.
func (b *Bootstrap) Clear() {
	if b == nil {
		return
	}
	clear(b.sessionKey)
	b.sessionKey = nil
}

func decodeBootstrap(input io.Reader) (*Bootstrap, error) {
	if input == nil {
		return nil, ErrInvalidBootstrap
	}
	limited := &io.LimitedReader{R: input, N: maxBootstrapJSONBytes + 1 + sessionKeyBytes + 1}
	reader := bufio.NewReaderSize(limited, maxBootstrapJSONBytes+1)
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) <= 1 || len(line) > maxBootstrapJSONBytes+1 {
		clear(line)
		return nil, ErrInvalidBootstrap
	}
	line = line[:len(line)-1]
	defer clear(line)
	fields, err := decodeExactObject(line)
	if err != nil {
		return nil, ErrInvalidBootstrap
	}
	expected := []string{
		"schema_version", "epoch_id", "container_id", "uid", "gid",
		"state_directory", "session_key_length",
	}
	if len(fields) != len(expected) {
		return nil, ErrInvalidBootstrap
	}
	for _, field := range expected {
		if _, ok := fields[field]; !ok {
			return nil, ErrInvalidBootstrap
		}
	}
	document := bootstrapDocument{}
	if !decodeInteger(fields["schema_version"], &document.SchemaVersion) ||
		!decodeString(fields["epoch_id"], &document.EpochID) ||
		!decodeString(fields["container_id"], &document.ContainerID) ||
		!decodeInteger(fields["uid"], &document.UID) ||
		!decodeInteger(fields["gid"], &document.GID) ||
		!decodeString(fields["state_directory"], &document.StateDirectory) ||
		!decodeInteger(fields["session_key_length"], &document.SessionKeyLength) ||
		validateBootstrapDocument(document) != nil {
		return nil, ErrInvalidBootstrap
	}
	canonical, err := json.Marshal(document)
	if err != nil || !bytes.Equal(canonical, line) {
		clear(canonical)
		return nil, ErrInvalidBootstrap
	}
	clear(canonical)
	key := make([]byte, sessionKeyBytes)
	if _, err := io.ReadFull(reader, key); err != nil {
		clear(key)
		return nil, ErrInvalidBootstrap
	}
	var trailing [1]byte
	if count, trailingErr := reader.Read(trailing[:]); count != 0 || !errors.Is(trailingErr, io.EOF) {
		clear(key)
		return nil, ErrInvalidBootstrap
	}
	return &Bootstrap{document: document, sessionKey: key}, nil
}

func validateBootstrapDocument(document bootstrapDocument) error {
	if document.SchemaVersion != bootstrapSchemaVersion ||
		document.SessionKeyLength != sessionKeyBytes ||
		!validEpochID(document.EpochID) || !containerIDPattern.MatchString(document.ContainerID) ||
		document.UID <= 0 || document.GID < 0 ||
		document.StateDirectory == "" || strings.IndexByte(document.StateDirectory, 0) >= 0 ||
		!filepath.IsAbs(document.StateDirectory) || filepath.Clean(document.StateDirectory) != document.StateDirectory {
		return ErrInvalidBootstrap
	}
	return nil
}

func validEpochID(value string) bool {
	if !strings.HasPrefix(value, epochPrefix) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, epochPrefix))
	if err != nil || len(raw) != epochBytes {
		return false
	}
	canonical := epochPrefix + base64.RawURLEncoding.EncodeToString(raw)
	clear(raw)
	return value == canonical
}

// ValidEpochID reports whether value is the canonical non-secret identifier
// accepted by both the lifecycle controller and the private companion.
func ValidEpochID(value string) bool { return validEpochID(value) }

func epochRaw(value string) ([]byte, error) {
	if !validEpochID(value) {
		return nil, ErrInvalidBootstrap
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, epochPrefix))
	if err != nil || len(raw) != epochBytes {
		return nil, ErrInvalidBootstrap
	}
	return raw, nil
}

func decodeExactObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalidBootstrap
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, nameErr := decoder.Token()
		name, ok := nameToken.(string)
		if nameErr != nil || !ok {
			return nil, ErrInvalidBootstrap
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrInvalidBootstrap
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, ErrInvalidBootstrap
		}
		fields[name] = raw
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalidBootstrap
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidBootstrap
	}
	return fields, nil
}

func decodeString(raw json.RawMessage, destination *string) bool {
	return json.Unmarshal(raw, destination) == nil
}

func decodeInteger(raw json.RawMessage, destination *int) bool {
	text := string(raw)
	if text == "" || strings.ContainsAny(text, ".eE+-") {
		return false
	}
	value, err := strconv.ParseUint(text, 10, 31)
	if err != nil {
		return false
	}
	*destination = int(value)
	return true
}
