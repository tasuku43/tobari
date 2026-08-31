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
	runs              []runnerCall
	outputs           []runnerCall
	inspectCalls      int
	metadata          string
	server            string
	firstInspectErr   error
	inspectDiagnostic string
	runErr            error
	runOutput         string
}

func (r *gatewayImageRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "pull" {
		r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
		return nil
	}
	if len(args) > 0 && args[0] == "version" {
		_, _ = io.WriteString(stdout, r.server)
		return nil
	}
	if len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
		r.inspectCalls++
		if r.inspectCalls == 1 && r.firstInspectErr != nil {
			diagnostic := r.inspectDiagnostic
			if diagnostic == "" {
				diagnostic = "Error: No such image: " + args[len(args)-1]
			}
			_, _ = io.WriteString(stderr, diagnostic)
			return r.firstInspectErr
		}
		if len(args) > 3 && args[3] == "{{.Id}}" {
			_, _ = io.WriteString(stdout, testGatewayDigest)
		} else {
			_, _ = io.WriteString(stdout, r.metadata)
		}
		return nil
	}
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if r.runOutput != "" {
		_, _ = io.WriteString(stderr, r.runOutput)
	}
	return r.runErr
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
	if _, err := runtime.verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true); err != nil {
		t.Fatal(err)
	}
	if runner.inspectCalls != 2 {
		t.Fatalf("image inspect calls = %d, want pull followed by re-inspect", runner.inspectCalls)
	}
	if len(runner.outputs) == 0 || runner.outputs[0].args[0] != "pull" {
		t.Fatalf("preflight output calls = %+v", runner.outputs)
	}
}

func TestVerifyGatewayImageRejectsUnreviewedAndOversizedVolumeMetadata(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		metadata string
	}{
		{name: "extra volume", metadata: strings.Replace(gatewayMetadata("arm64", ""), `"Entrypoint":["/opt/tobari/entrypoint.sh"]`, `"Entrypoint":["/opt/tobari/entrypoint.sh"],"Volumes":{"/unreviewed":{}}`, 1)},
		{name: "oversize", metadata: strings.Repeat("x", componentImageInspectLimit)},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &gatewayImageRunner{metadata: test.metadata, server: `{"Os":"linux","Arch":"arm64"}`}
			_, err := (&Runtime{runner: runner}).verifyGatewayImage(context.Background(), "tobari-gateway:test", false)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "gateway_image_unavailable" {
				if test.name == "extra volume" && ok && public.Code == "gateway_image_incompatible" {
					return
				}
				t.Fatalf("error = %v public=%+v", err, public)
			}
		})
	}
}

func TestEnsureLocalGatewayImageDoesNotBuildAfterAmbiguousInspectFailure(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{firstInspectErr: errors.New("daemon unavailable"), inspectDiagnostic: "daemon unavailable"}
	runtime := &Runtime{stateDirectory: t.TempDir(), runner: runner}
	err := runtime.ensureLocalGatewayImage(context.Background(), "tobari-gateway:test")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "gateway_image_unavailable" {
		t.Fatalf("error = %v public=%+v", err, public)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("ambiguous inspect triggered build: %+v", runner.runs)
	}
}

func TestVerifyDigestGatewayDoesNotPullAfterAmbiguousInspectFailure(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{firstInspectErr: errors.New("permission denied"), inspectDiagnostic: "permission denied"}
	_, err := (&Runtime{runner: runner}).verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "gateway_image_unavailable" {
		t.Fatalf("error = %v public=%+v", err, public)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatalf("ambiguous inspect triggered pull: %+v", runner.outputs)
		}
	}
}

func TestResolveOPAImageRejectsDeclaredVolume(t *testing.T) {
	t.Parallel()
	metadata := `{"Id":"sha256:` + strings.Repeat("2", 64) + `","RepoDigests":[],"Architecture":"arm64","Os":"linux","Config":{"Labels":{},"Volumes":{"/leak":{}}}}`
	runner := &gatewayImageRunner{metadata: metadata}
	_, err := (&Runtime{runner: runner}).resolveVolumeSafeOPAImageID(context.Background(), "openpolicyagent/opa:test")
	public, ok := fault.PublicCopy(err)
	if err == nil || !ok || public.Code != "cluster_resource_conflict" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatal("OPA image declared volume was accepted")
	}
}

func TestResolveOPAImageClassifiesOversizedMetadataAsIncompatible(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{metadata: strings.Repeat("x", componentImageInspectLimit)}
	_, err := (&Runtime{runner: runner}).resolveVolumeSafeOPAImageID(context.Background(), "openpolicyagent/opa:test")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "cluster_resource_conflict" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("error = %v public=%+v", err, public)
	}
}

func TestVerifyGatewayImageRejectsOldAPIBeforeEngineMutation(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: gatewayMetadata("arm64", "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest),
		server:   `{"Os":"linux","Arch":"arm64"}`,
	}
	runner.metadata = strings.Replace(runner.metadata, `"io.tobari.gateway-api":"1"`, `"io.tobari.gateway-api":"2"`, 1)
	runtime := &Runtime{runner: runner}
	_, err := runtime.verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true)
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
	_, err := runtime.verifyGatewayImage(context.Background(), "ghcr.io/tasuku43/tobari/gateway@"+testGatewayDigest, true)
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
			runtimeImage: "tobari-runtime:test",
			gateway:      sharedImageSelection{Image: "tobari-gateway:dev", RequireDigest: false},
		},
	}
	image, identity, err := runtime.prepareGatewayImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if image != "tobari-gateway:dev" {
		t.Fatalf("Gateway image = %q", image)
	}
	if identity != testGatewayDigest {
		t.Fatalf("Gateway image identity = %q", identity)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatalf("local Gateway image was pulled: %v", runner.outputs)
		}
	}
}

func TestPrepareGatewayImageBuildsMissingEmbeddedGatewayBeforeValidation(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata:        gatewayMetadata("arm64", ""),
		server:          `{"Os":"linux","Arch":"arm64"}`,
		firstInspectErr: errors.New("image is not present"),
	}
	runtime := &Runtime{
		stateDirectory: t.TempDir(),
		runner:         runner,
		images: testImageResolver{
			gateway: sharedImageSelection{Image: "tobari-gateway:base-example", BuildIfMissing: true},
		},
	}
	image, identity, err := runtime.prepareGatewayImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if image != "tobari-gateway:base-example" || identity != testGatewayDigest || len(runner.runs) != 1 {
		t.Fatalf("Gateway preparation = image %q identity %q runs %+v", image, identity, runner.runs)
	}
	joined := strings.Join(runner.runs[0].args, "\n")
	for _, required := range []string{"buildx", "build", "--progress=plain", "--load", "--tag", image, "gateway/Dockerfile", "MITMPROXY_IMAGE=mitmproxy/mitmproxy@sha256:"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("Gateway build argv %q lacks %q", joined, required)
		}
	}
}

func TestPrepareGatewayImageReusesExistingEmbeddedGateway(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: gatewayMetadata("arm64", ""),
		server:   `{"Os":"linux","Arch":"arm64"}`,
	}
	runtime := &Runtime{
		stateDirectory: t.TempDir(),
		runner:         runner,
		images: testImageResolver{
			gateway: sharedImageSelection{Image: "tobari-gateway:base-example", BuildIfMissing: true},
		},
	}
	if _, _, err := runtime.prepareGatewayImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("existing Gateway triggered builds: %+v", runner.runs)
	}
}

func TestPrepareGatewayImageReportsEmbeddedBuildFailure(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		firstInspectErr: errors.New("image is not present"),
		runErr:          errors.New("build failed"),
		runOutput:       "synthetic build diagnostic",
	}
	runtime := &Runtime{
		stateDirectory: t.TempDir(),
		runner:         runner,
		images: testImageResolver{
			gateway: sharedImageSelection{Image: "tobari-gateway:base-example", BuildIfMissing: true},
		},
	}
	_, _, err := runtime.prepareGatewayImage(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "gateway_image_build_failed" || public.Retryable {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
	cause := errors.Unwrap(err)
	if cause == nil || !strings.Contains(cause.Error(), "synthetic build diagnostic") {
		t.Fatalf("build diagnostic was discarded: %v (cause %v)", err, cause)
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
	return `{"Id":"` + testGatewayDigest + `","RepoDigests":` + repoDigests + `,"Architecture":"` + architecture + `","Os":"linux","Config":{"User":"1000:1000","Labels":{"io.tobari.gateway-api":"1","io.tobari.gateway-role":"enforcement"},"Entrypoint":["/opt/tobari/entrypoint.sh"]}}`
}

func componentMetadataFixture(imageID, component string) string {
	config := `"Labels":{}`
	switch component {
	case "gateway":
		config = `"User":"1000:1000","Labels":{"io.tobari.gateway-api":"1","io.tobari.gateway-role":"enforcement"},"Entrypoint":["/opt/tobari/entrypoint.sh"]`
	case "auth-broker":
		config = `"User":"1000:1000","Labels":{"io.tobari.auth-broker-api":"1","io.tobari.auth-broker-role":"credential-resolution"},"Entrypoint":["/opt/tobari/entrypoint.sh"]`
	}
	return `{"Id":"` + imageID + `","RepoDigests":[],"Architecture":"arm64","Os":"linux","Config":{` + config + `}}`
}
