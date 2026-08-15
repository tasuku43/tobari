package credentialhost

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func pupNativeFileFixtures(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	client := pupClientCredentials{
		ClientID: "client-example-123", ClientName: pupClientName,
		RedirectURIs: []string{"http://127.0.0.1:8000/oauth/callback"},
		RegisteredAt: 1_800_000_000, Site: PupSite,
	}
	token := pupTokenSet{
		AccessToken: "dummy-access-token", RefreshToken: "dummy-refresh-token",
		TokenType: "Bearer", ExpiresIn: 3600, IssuedAt: 1_800_000_001,
		Scope: "dashboards_read metrics_read", ClientID: client.ClientID,
	}
	encode := func(value any) []byte {
		content, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	return encode(client), encode(map[string]pupTokenSet{"__default__": token}),
		encode([]map[string]any{{"site": PupSite, "org": nil}})
}

func TestNewPupStateFromNativeFilesCanonicalizesReviewedUS1Session(t *testing.T) {
	client, token, sessions := pupNativeFileFixtures(t)
	state, err := NewPupStateFromNativeFiles(
		"/usr/local/bin/pup", strings.Repeat("d", 64), client, token, sessions,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Clear()
	if state.Site() != PupSite || state.DriverID() != PupDriverID ||
		state.DriverRevision() != strings.Repeat("d", 64) {
		t.Fatalf("pup state metadata = site=%q driver=%q revision=%q", state.Site(), state.DriverID(), state.DriverRevision())
	}
	encoded, err := state.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePupState(encoded)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Clear()
	if strings.Contains(state.String(), "access-token") || strings.Contains(state.GoString(), "refresh-token") {
		t.Fatal("pup state formatting exposed credential material")
	}
}

func TestNewPupStateFromNativeFilesRejectsSchemaAndSessionDrift(t *testing.T) {
	client, token, sessions := pupNativeFileFixtures(t)
	tests := map[string]struct {
		client   []byte
		token    []byte
		sessions []byte
	}{
		"unknown client field": {[]byte(`{"client_id":"leak"}`), token, sessions},
		"multiple token slots": {client, []byte(`{"__default__":{},"other":{}}`), sessions},
		"named org":            {client, token, []byte(`[{"site":"datadoghq.com","org":"other"}]`)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state, err := NewPupStateFromNativeFiles(
				"/usr/local/bin/pup", strings.Repeat("d", 64), test.client, test.token, test.sessions,
			)
			state.Clear()
			if !errors.Is(err, ErrInvalidPupState) {
				t.Fatalf("native pup state error = %v", err)
			}
		})
	}
}
