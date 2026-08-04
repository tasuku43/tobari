package dockerruntime

import (
	"context"
	"errors"
	"io"
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

func TestPrepareGatewayImageUsesInjectedLocalResolver(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: gatewayMetadata("arm64", ""),
		server:   `{"Os":"linux","Arch":"arm64"}`,
	}
	runtime := &Runtime{
		runner: runner,
		images: testImageResolver{
			runtimeImage: "tobari-runtime:dev",
			gateway:      gatewayImageSelection{Image: "tobari-gateway:dev", RequireDigest: false},
		},
	}
	image, err := runtime.prepareGatewayImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if image != "tobari-gateway:dev" {
		t.Fatalf("Gateway image = %q", image)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatalf("local Gateway image was pulled: %v", runner.outputs)
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
