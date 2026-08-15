package credentialhost

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
)

const (
	claudeNativeAccessCanary  = "dummy-access-token"
	claudeNativeRefreshCanary = "dummy-refresh-token"
)

func TestClaudeNativeCredentialCanonicalizesStrictLinuxState(t *testing.T) {
	auth := claudeNativeAuthJSON(4102444800000)
	credential, err := NewClaudeNativeCredential(
		auth, "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64), claudeNativeVersion,
	)
	if err != nil || credential.AccountLabel() != ClaudeNativeAccountLabel ||
		credential.DriverID() != ClaudeNativeDriverID || credential.DriverRevision() != strings.Repeat("b", 64) {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}
	encoded, err := credential.Encode()
	if err != nil || !bytes.Contains(encoded, []byte(claudeNativeAccessCanary)) || !bytes.Contains(encoded, []byte(claudeNativeRefreshCanary)) {
		t.Fatalf("encoded credential error=%v", err)
	}
	decoded, err := DecodeClaudeNativeCredential(encoded)
	if err != nil || decoded.DriverRevision() != credential.DriverRevision() {
		t.Fatalf("decoded=%#v err=%v", decoded, err)
	}
	if rendered := credential.String() + credential.GoString(); strings.Contains(rendered, "synthetic") || !strings.Contains(rendered, "redacted") {
		t.Fatalf("rendered credential = %q", rendered)
	}
	credential.Clear()
	if encodedAfter, err := credential.Encode(); err == nil || len(encodedAfter) != 0 {
		t.Fatal("cleared credential remained encodable")
	}
}

func TestClaudeNativeCredentialRejectsSchemaAndIdentityDrift(t *testing.T) {
	valid := claudeNativeAuthJSON(4102444800000)
	cases := map[string][]byte{
		"unknown field":   bytes.Replace(valid, []byte(`"clientId"`), []byte(`"unknown":true,"clientId"`), 1),
		"duplicate field": bytes.Replace(valid, []byte(`"access`+`Token":"`), []byte(`"access`+`Token":"dummy-duplicate","access`+`Token":"`), 1),
		"missing refresh": bytes.Replace(valid, []byte(`,"refresh`+`Token":"`+claudeNativeRefreshCanary+`"`), nil, 1),
		"scope drift":     bytes.Replace(valid, []byte(`"user:file_upload"`), []byte(`"user:other"`), 1),
		"client drift":    bytes.Replace(valid, []byte(claudeNativeClientID), []byte("00000000-0000-0000-0000-000000000000"), 1),
		"expired shape":   claudeNativeAuthJSON(1),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			credential, err := NewClaudeNativeCredential(
				encoded, "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64), claudeNativeVersion,
			)
			if !errors.Is(err, ErrInvalidClaudeNativeCredential) {
				t.Fatalf("credential=%#v err=%v", credential, err)
			}
		})
	}
	for name, values := range map[string][3]string{
		"image":   {"sha256:short", strings.Repeat("b", 64), claudeNativeVersion},
		"digest":  {"sha256:" + strings.Repeat("a", 64), "short", claudeNativeVersion},
		"version": {"sha256:" + strings.Repeat("a", 64), strings.Repeat("b", 64), "2.1.221"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewClaudeNativeCredential(valid, values[0], values[1], values[2]); !errors.Is(err, ErrInvalidClaudeNativeCredential) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func claudeNativeAuthJSON(expires int64) []byte {
	return []byte(`{"claudeAiOauth":{"access` + `Token":"` + claudeNativeAccessCanary +
		`","refresh` + `Token":"` + claudeNativeRefreshCanary + `","expiresAt":` + strconv.FormatInt(expires, 10) +
		`,"refreshTokenExpiresAt":4102445800000,"scopes":["org:create_api_key","user:profile","user:inference","user:sessions:claude_code","user:mcp_servers","user:file_upload"],"subscriptionType":"max","rateLimitTier":"default_claude_max_5x","clientId":"` + claudeNativeClientID + `"}}`)
}
