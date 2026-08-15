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
	for _, providerMetadata := range [][]byte{[]byte("refreshTokenExpiresAt"), []byte("subscriptionType"), []byte("rateLimitTier"), []byte("clientId"), []byte("claudeAiOauth")} {
		if bytes.Contains(encoded, providerMetadata) {
			t.Fatalf("canonical state retained provider metadata %q", providerMetadata)
		}
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
	cases := map[string]struct {
		encoded []byte
		stage   ClaudeCredentialCaptureStage
	}{
		"duplicate field": {bytes.Replace(valid, []byte(`"access`+`Token":"`), []byte(`"access`+`Token":"dummy-duplicate","access`+`Token":"`), 1), ClaudeCaptureDocumentJSON},
		"missing refresh": {bytes.Replace(valid, []byte(`,"refresh`+`Token":"`+claudeNativeRefreshCanary+`"`), nil, 1), ClaudeCaptureOAuthCoreFields},
		"invalid scope":   {bytes.Replace(valid, []byte(`"user:file_upload"`), []byte(`"user other"`), 1), ClaudeCaptureOAuthScopeSet},
		"duplicate scope": {bytes.Replace(valid, []byte(`"user:file_upload"`), []byte(`"user:profile"`), 1), ClaudeCaptureOAuthScopeSet},
		"expired shape":   {claudeNativeAuthJSON(1), ClaudeCaptureOAuthExpiry},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			credential, err := NewClaudeNativeCredential(
				test.encoded, "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64), claudeNativeVersion,
			)
			if !errors.Is(err, ErrInvalidClaudeNativeCredential) {
				t.Fatalf("credential=%#v err=%v", credential, err)
			}
			var diagnostic *ClaudeCredentialCaptureError
			if !errors.As(err, &diagnostic) || diagnostic.DiagnosticStage() != test.stage {
				t.Fatalf("diagnostic=%#v error=%v", diagnostic, err)
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

func TestClaudeNativeCredentialTreatsDynamicGrantedScopesAsASet(t *testing.T) {
	reordered := bytes.Replace(
		claudeNativeAuthJSON(4102444800000),
		[]byte(`"org:create_api_key","user:profile","user:inference","user:sessions:claude_code","user:mcp_servers","user:file_upload"`),
		[]byte(`"future:capability","user:inference"`),
		1,
	)
	credential, err := NewClaudeNativeCredential(
		reordered, "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64), claudeNativeVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := credential.Encode()
	if err != nil {
		t.Fatal(err)
	}
	canonicalOrder := `"scopes":["future:capability","user:inference"]`
	if !bytes.Contains(encoded, []byte(canonicalOrder)) {
		t.Fatalf("canonical state did not normalize scope order: %s", encoded)
	}
}

func TestClaudeNativeCredentialDropsProviderOwnedOptionalMetadata(t *testing.T) {
	valid := claudeNativeAuthJSON(4102444800000)
	withMetadata := bytes.Replace(
		valid,
		[]byte(`}}`),
		[]byte(`,"clientId":"9d1c250a-e61b-44d9-88ed-5944d1962f5e","providerFuture":{"secret":"dummy-provider-metadata-canary"}},"providerRoot":true}`),
		1,
	)
	credential, err := NewClaudeNativeCredential(
		withMetadata, "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64), claudeNativeVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := credential.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, metadata := range [][]byte{[]byte("clientId"), []byte("providerFuture"), []byte("providerRoot"), []byte("dummy-provider-metadata-canary")} {
		if bytes.Contains(encoded, metadata) {
			t.Fatalf("canonical state retained provider metadata %q", metadata)
		}
	}
}

func claudeNativeAuthJSON(expires int64) []byte {
	return []byte(`{"claudeAiOauth":{"access` + `Token":"` + claudeNativeAccessCanary +
		`","refresh` + `Token":"` + claudeNativeRefreshCanary + `","expiresAt":` + strconv.FormatInt(expires, 10) +
		`,"refreshTokenExpiresAt":4102445800000,"scopes":["org:create_api_key","user:profile","user:inference","user:sessions:claude_code","user:mcp_servers","user:file_upload"],"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`)
}
