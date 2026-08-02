package dockerruntime

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const testGatewayDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

type gatewayImageRunner struct {
	runs            []runnerCall
	outputs         []runnerCall
	inspectCalls    int
	metadata        string
	server          string
	firstInspectErr error
}

func (r *gatewayImageRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}

func (r *gatewayImageRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "image" {
		r.inspectCalls++
		if r.inspectCalls == 1 && r.firstInspectErr != nil {
			return nil, r.firstInspectErr
		}
		return []byte(r.metadata), nil
	}
	if len(args) > 0 && args[0] == "version" {
		return []byte(r.server), nil
	}
	if len(args) > 0 && args[0] == "pull" {
		return []byte("pulled"), nil
	}
	return nil, nil
}

func TestVerifyGatewayImagePullsAndChecksImmutableContract(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata:        gatewayMetadata("arm64", "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest),
		server:          `{"Os":"linux","Arch":"aarch64"}`,
		firstInspectErr: errors.New("image is not present"),
	}
	runtime := &Runtime{runner: runner}
	if err := runtime.verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true); err != nil {
		t.Fatal(err)
	}
	if runner.inspectCalls != 2 {
		t.Fatalf("image inspect calls = %d, want pull followed by re-inspect", runner.inspectCalls)
	}
	if len(runner.outputs) < 2 || runner.outputs[1].args[0] != "pull" {
		t.Fatalf("preflight output calls = %+v", runner.outputs)
	}
}

func TestVerifyGatewayImageRejectsContractBeforeEngineMutation(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: gatewayMetadata("arm64", "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest),
		server:   `{"Os":"linux","Arch":"arm64"}`,
	}
	runner.metadata = strings.Replace(runner.metadata, `"io.tobari.gateway-api":"1"`, `"io.tobari.gateway-api":"2"`, 1)
	runtime := &Runtime{runner: runner}
	err := runtime.verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "gateway_image_incompatible" {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatal("incompatible image was pulled")
		}
	}
}

func TestVerifyGatewayImageRejectsEngineArchitectureMismatch(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: gatewayMetadata("arm64", "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest),
		server:   `{"Os":"linux","Arch":"amd64"}`,
	}
	runtime := &Runtime{runner: runner}
	err := runtime.verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "gateway_image_incompatible" {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
}

func TestPrepareGatewayImageSourceBuildUsesEmbeddedSnapshotExplicitly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &gatewayImageRunner{
		metadata: gatewayMetadata("arm64", ""),
		server:   `{"Os":"linux","Arch":"arm64"}`,
	}
	runtime, err := newRuntime(
		filepath.Join(root, "config"), filepath.Join(root, "state"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	image, err := runtime.prepareGatewayImage(context.Background(), state, []string{"PATH=/usr/bin"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if image != "tobari-gateway:"+state.AssetVersion {
		t.Fatalf("source image = %q", image)
	}
	if len(runner.runs) != 1 || runner.runs[0].args[0] != "build" {
		t.Fatalf("source build calls = %+v", runner.runs)
	}
	if !containsArgPair(runner.runs[0].args, "--tag", image) {
		t.Fatalf("source build args = %+v", runner.runs[0].args)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatal("source mode pulled the official image")
		}
	}
}

func TestReplaceEnvironmentValueDoesNotPermitDuplicateImageSelectors(t *testing.T) {
	t.Parallel()
	environment := replaceEnvironmentValue(
		[]string{"PATH=/usr/bin", "TOBARI_GATEWAY_IMAGE=untrusted", "TOBARI_GATEWAY_IMAGE=old"},
		"TOBARI_GATEWAY_IMAGE", "verified",
	)
	if strings.Count(strings.Join(environment, "\n"), "TOBARI_GATEWAY_IMAGE=") != 1 {
		t.Fatalf("environment = %v", environment)
	}
	if environment[len(environment)-1] != "TOBARI_GATEWAY_IMAGE=verified" {
		t.Fatalf("environment tail = %v", environment[len(environment)-1])
	}
}

func gatewayMetadata(architecture, repoDigest string) string {
	repoDigests := "[]"
	if repoDigest != "" {
		repoDigests = `["` + repoDigest + `"]`
	}
	return `{"RepoDigests":` + repoDigests + `,"Architecture":"` + architecture + `","Os":"linux","Config":{"User":"1000:1000","Labels":{"io.tobari.gateway-api":"1","io.tobari.gateway-role":"enforcement"},"Entrypoint":["/opt/tobari/entrypoint.sh"]}}`
}

func containsArgPair(args []string, key, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return true
		}
	}
	return false
}
