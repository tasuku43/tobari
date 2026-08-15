package credentialhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

var (
	ErrPupLoginFailed  = errors.New("pup OAuth login failed")
	ErrInvalidPupState = errors.New("pup OAuth state is invalid")
	ErrPupLoginSetup   = errors.New("pup OAuth login setup failed")
	ErrPupLoginCleanup = errors.New("pup OAuth login cleanup failed")
	ErrPupOutputLimit  = errors.New("pup OAuth login output exceeded its limit")
)

// NewPupStateFromNativeFiles parses the exact three file-backed values emitted
// by a pup executable that passed the isolated native-login contract and
// canonicalizes them into Tobari-owned opaque state. Callers retain ownership
// of the byte slices and must clear them.
func NewPupStateFromNativeFiles(
	executablePath, executableDigest string,
	clientBytes, tokenBytes, sessionsBytes []byte,
) (PupState, error) {
	var client pupClientCredentials
	if err := decodeExactJSON(clientBytes, &client); err != nil {
		return PupState{}, ErrInvalidPupState
	}
	var tokens map[string]pupTokenSet
	if err := decodeExactJSON(tokenBytes, &tokens); err != nil || len(tokens) != 1 {
		return PupState{}, ErrInvalidPupState
	}
	token, ok := tokens["__default__"]
	if !ok {
		return PupState{}, ErrInvalidPupState
	}
	var sessions []struct {
		Site    string  `json:"site"`
		Org     *string `json:"org"`
		OrgUUID *string `json:"org_uuid,omitempty"`
	}
	if err := decodeExactJSON(sessionsBytes, &sessions); err != nil || len(sessions) != 1 ||
		sessions[0].Site != PupSite || sessions[0].Org != nil {
		return PupState{}, ErrInvalidPupState
	}
	if sessions[0].OrgUUID != nil && !validUUID(*sessions[0].OrgUUID) {
		return PupState{}, ErrInvalidPupState
	}
	state, err := newPupState(executablePath, executableDigest, client, token)
	if err != nil {
		return PupState{}, ErrInvalidPupState
	}
	return state, nil
}

func decodeExactJSON(encoded []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidPupState
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, current := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if current != '-' {
				return false
			}
			continue
		}
		if !((current >= '0' && current <= '9') || (current >= 'a' && current <= 'f') || (current >= 'A' && current <= 'F')) {
			return false
		}
	}
	return true
}
