package credentialhost

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCacheName = "0123456789abcdef0123456789abcdef01234567.json"

func testProfile() ProfileConfig {
	return ProfileConfig{
		StartURL:  "https://example.awsapps.com/start",
		SSORegion: "us-east-1",
		AccountID: "123456789012",
		RoleName:  "Developer-Role",
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aws")
	if err := os.WriteFile(path, []byte("synthetic aws executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func testCacheContent(token string) []byte {
	type cacheFixture struct {
		StartURL   string `json:"startUrl"`
		Credential string `json:"accessToken"`
	}
	content, err := json.Marshal(cacheFixture{
		StartURL:   "https://example.awsapps.com/start",
		Credential: token,
	})
	if err != nil {
		panic(err)
	}
	return content
}

func writeCacheFile(t *testing.T, directory, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStateRoundTripIsCanonicalAndRedacted(t *testing.T) {
	executable := testExecutable(t)
	canonical, digest, err := resolveExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	secret := "cache-secret-value"
	cache := []stateCacheFile{{
		Name:             testCacheName,
		ContentBase64URL: encodeCacheForTest(testCacheContent(secret)),
	}}
	state, err := newState(testProfile(), canonical, digest, cache)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := state.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeState(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := decoded.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("state encoding is not canonical:\n%s\n%s", encoded, reencoded)
	}
	if decoded.Profile() != testProfile() {
		t.Fatalf("profile = %+v", decoded.Profile())
	}
	if decoded.DriverRevision() != digest {
		t.Fatalf("driver revision = %q, want %q", decoded.DriverRevision(), digest)
	}
	for _, rendered := range []string{fmt.Sprintf("%v", state), fmt.Sprintf("%#v", state)} {
		if strings.Contains(rendered, secret) || !strings.Contains(rendered, "redacted") {
			t.Fatalf("state formatting was not redacted: %q", rendered)
		}
	}
	if !bytes.Contains(encoded, []byte(`"schema_version":1`)) ||
		!bytes.Contains(encoded, []byte(`"path":"`+canonical+`"`)) ||
		!bytes.Contains(encoded, []byte(`"sha256":"`+digest+`"`)) {
		t.Fatalf("state does not bind schema/path/digest: %s", encoded)
	}
	if _, err := DecodeState(append([]byte(" "), encoded...)); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("noncanonical state error = %v", err)
	}
	unknown := append(encoded[:len(encoded)-1], []byte(`,"unknown":true}`)...)
	if _, err := DecodeState(unknown); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("unknown-field state error = %v", err)
	}
}

func TestResolveExecutableCanonicalizesSymlink(t *testing.T) {
	target := testExecutable(t)
	link := filepath.Join(t.TempDir(), "aws-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	canonical, digest, err := resolveExecutable(link)
	if err != nil {
		t.Fatal(err)
	}
	wantCanonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != wantCanonical || !digestPattern.MatchString(digest) {
		t.Fatalf("canonical=%q digest=%q", canonical, digest)
	}
}

func TestProfileRejectsUnreviewedAccessPortalAuthorities(t *testing.T) {
	for _, startURL := range []string{
		"http://example.awsapps.com/start",
		"https://example.com/start",
		"https://example.awsapps.com/start/extra",
		"https://example.awsapps.com/start?next=secret",
		"https://example.awsapps.com.evil.example/start",
		"https://EXAMPLE.awsapps.com/start",
	} {
		profile := testProfile()
		profile.StartURL = startURL
		if err := validateProfile(profile); !errors.Is(err, ErrInvalidProfile) {
			t.Fatalf("start URL %q error = %v", startURL, err)
		}
	}
}

func TestProfileRejectsNonCommercialSSORegions(t *testing.T) {
	for _, region := range []string{
		"cn-north-1",
		"us-gov-west-1",
		"us-iso-east-1",
		"eu-isoe-west-1",
	} {
		profile := testProfile()
		profile.SSORegion = region
		if err := validateProfile(profile); !errors.Is(err, ErrInvalidProfile) {
			t.Fatalf("SSO region %q error = %v", region, err)
		}
	}
}

func TestPackCacheRejectsSymlinkOversizeNonregularAndInvalidJSON(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"symlink": func(t *testing.T, cache string) {
			target := filepath.Join(t.TempDir(), "target.json")
			if err := os.WriteFile(target, []byte(`{"token":"secret"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(cache, testCacheName)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		},
		"oversize": func(t *testing.T, cache string) {
			writeCacheFile(t, cache, testCacheName, append([]byte(`{"padding":"`), append(bytes.Repeat([]byte("x"), maxCacheFileBytes), []byte(`"}`)...)...))
		},
		"nonregular": func(t *testing.T, cache string) {
			if err := os.Mkdir(filepath.Join(cache, testCacheName), 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"invalid JSON": func(t *testing.T, cache string) {
			writeCacheFile(t, cache, testCacheName, []byte(`{"token":`))
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			cache := t.TempDir()
			if err := os.Chmod(cache, 0o700); err != nil {
				t.Fatal(err)
			}
			arrange(t, cache)
			if _, err := packCache(cache); !errors.Is(err, ErrInvalidCache) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDecodeStateRejectsTraversalAndInvalidJSON(t *testing.T) {
	executable := testExecutable(t)
	canonical, digest, err := resolveExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := newState(testProfile(), canonical, digest, []stateCacheFile{{
		Name:             testCacheName,
		ContentBase64URL: encodeCacheForTest(testCacheContent("secret")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := clonePayload(valid.payload)
	payload.Cache[0].Name = "../escape.json"
	traversal, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeState(traversal); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("traversal error = %v", err)
	}
	for _, invalid := range [][]byte{[]byte(`{`), []byte(`null`), append(mustEncodeState(t, valid), []byte(`{}`)...)} {
		if _, err := DecodeState(invalid); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("invalid JSON %q error = %v", invalid, err)
		}
	}
}

func encodeCacheForTest(content []byte) string {
	return base64.RawURLEncoding.EncodeToString(content)
}

func mustEncodeState(t *testing.T, state State) []byte {
	t.Helper()
	encoded, err := state.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
