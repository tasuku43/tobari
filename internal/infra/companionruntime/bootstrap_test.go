package companionruntime

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBootstrapRoundTripUsesCanonicalEpochDerivationAndRedaction(t *testing.T) {
	t.Parallel()
	rootKey, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	epochRaw, _ := hex.DecodeString("1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100")
	bootstrap, err := NewBootstrap(
		bytes.NewReader(epochRaw), rootKey, strings.Repeat("a", 64), 501, 20, "/tmp/tobari-state",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Clear()
	if bootstrap.EpochID() != "companion-e1_Hx4dHBsaGRgXFhUUExIREA8ODQwLCgkIBwYFBAMCAQA" {
		t.Fatalf("epoch = %q", bootstrap.EpochID())
	}
	if got := hex.EncodeToString(bootstrap.sessionKey); got != "0d7f38e34b1bda5b2e9d9d61e3e89acc3faa736e5530b49576013bf38f062a0e" {
		t.Fatalf("derived epoch key = %s", got)
	}
	encoded, err := bootstrap.Encode()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(encoded)
	decoded, err := decodeBootstrap(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Clear()
	if !reflect.DeepEqual(decoded.document, bootstrap.document) || !bytes.Equal(decoded.sessionKey, bootstrap.sessionKey) {
		t.Fatalf("decoded bootstrap does not match")
	}
	formatted := fmt.Sprintf("%s %#v", bootstrap, bootstrap)
	for _, secret := range []string{hex.EncodeToString(rootKey), hex.EncodeToString(bootstrap.sessionKey)} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("formatted bootstrap exposed secret %q: %s", secret, formatted)
		}
	}
}

func TestDecodeBootstrapRejectsAmbiguousOrTrailingInput(t *testing.T) {
	t.Parallel()
	validJSON := `{"schema_version":1,"epoch_id":"companion-e1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","container_id":"` + strings.Repeat("a", 64) + `","uid":501,"gid":20,"state_directory":"/tmp/tobari-state","session_key_length":32}`
	key := strings.Repeat("k", 32)
	for name, input := range map[string]string{
		"missing newline":    validJSON + key,
		"trailing byte":      validJSON + "\n" + key + "x",
		"short key":          validJSON + "\n" + key[:31],
		"duplicate field":    strings.Replace(validJSON, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1) + "\n" + key,
		"unknown field":      strings.TrimSuffix(validJSON, "}") + `,"unknown":true}` + "\n" + key,
		"noncanonical JSON":  strings.Replace(validJSON, `,"epoch_id"`, `, "epoch_id"`, 1) + "\n" + key,
		"noncanonical epoch": strings.Replace(validJSON, "companion-e1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "companion-e1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", 1) + "\n" + key,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if decoded, err := decodeBootstrap(strings.NewReader(input)); err == nil {
				decoded.Clear()
				t.Fatal("decodeBootstrap accepted invalid input")
			}
		})
	}
}

func TestDockerEnvironmentUsesOnlyClosedConnectionAllowlist(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"PATH": "/usr/bin", "HOME": "/synthetic/user", "DOCKER_HOST": "unix:///run/docker.sock",
		"DOCKER_CONTEXT": "desktop-linux", "DOCKER_CONFIG": "/synthetic/user/.docker",
		"DOCKER_TLS_VERIFY": "1", "DOCKER_CERT_PATH": "/safe/certs", "DOCKER_API_VERSION": "1.51",
		"DOCKER_DEFAULT_PLATFORM": "linux/arm64", "AWS_ACCESS_KEY_ID": "forbidden",
		"GH_TOKEN": "forbidden", "HTTPS_PROXY": "forbidden", "LD_PRELOAD": "forbidden", "DYLD_INSERT_LIBRARIES": "forbidden",
	}
	got := dockerEnvironment(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	want := []string{
		"PATH=/usr/bin", "HOME=/synthetic/user", "DOCKER_HOST=unix:///run/docker.sock",
		"DOCKER_CONTEXT=desktop-linux", "DOCKER_CONFIG=/synthetic/user/.docker",
		"DOCKER_TLS_VERIFY=1", "DOCKER_CERT_PATH=/safe/certs", "DOCKER_API_VERSION=1.51",
		"DOCKER_DEFAULT_PLATFORM=linux/arm64",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
	joined := strings.Join(got, "\n")
	for _, forbidden := range []string{"AWS_", "GH_", "TOKEN", "PROXY", "LD_", "DYLD_"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("environment contains %q: %s", forbidden, joined)
		}
	}
}
