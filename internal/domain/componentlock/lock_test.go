package componentlock

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLockRoundTripAndReferences(t *testing.T) {
	lock := Lock{SchemaVersion: 1, SourceRevision: strings.Repeat("a", 40),
		Gateway:    Component{Image: "ghcr.io/tasuku43/tobari/gateway", Digest: "sha256:" + strings.Repeat("1", 64), API: 1, Platforms: []string{"linux/amd64", "linux/arm64"}},
		AuthBroker: Component{Image: "ghcr.io/tasuku43/tobari/auth-broker", Digest: "sha256:" + strings.Repeat("2", 64), API: 1, Platforms: []string{"linux/amd64", "linux/arm64"}},
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Gateway.Reference() != lock.Gateway.Image+"@"+lock.Gateway.Digest {
		t.Fatalf("reference = %q", parsed.Gateway.Reference())
	}
}

func TestLockRejectsIncompleteOrNoncanonicalAuthority(t *testing.T) {
	base := `{"schema_version":1,"source_revision":"` + strings.Repeat("a", 40) + `","gateway":{"image":"ghcr.io/tasuku43/tobari/gateway","digest":"sha256:` + strings.Repeat("1", 64) + `","api":1,"platforms":["linux/amd64","linux/arm64"]},"auth_broker":{"image":"ghcr.io/tasuku43/tobari/auth-broker","digest":"sha256:` + strings.Repeat("2", 64) + `","api":1,"platforms":["linux/amd64","linux/arm64"]}}`
	for name, data := range map[string]string{
		"moving tag":        strings.Replace(base, `ghcr.io/tasuku43/tobari/gateway"`, `ghcr.io/tasuku43/tobari/gateway:main"`, 1),
		"partial platforms": strings.Replace(base, `["linux/amd64","linux/arm64"]`, `["linux/amd64"]`, 1),
		"unknown field":     strings.Replace(base, `"schema_version":1`, `"schema_version":1,"extra":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(data)); err == nil {
				t.Fatal("invalid lock accepted")
			}
		})
	}
}
