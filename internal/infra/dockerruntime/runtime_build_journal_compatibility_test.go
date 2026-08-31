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

type managedRuntimeCompatibilityRunner struct {
	inspection string
	args       []string
}

func TestRuntimeCompatibilityObservationIsBoundedAndClassifiesExactMissing(t *testing.T) {
	t.Run("overflow is incompatible", func(t *testing.T) {
		runner := &managedRuntimeCompatibilityRunner{inspection: strings.Repeat("x", 4097)}
		runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
		if err != nil {
			t.Fatal(err)
		}
		err = runtime.validateCompatibleImage(context.Background(), "example.invalid/runtime:test")
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "incompatible_image" {
			t.Fatalf("overflow classification = %v / %+v", err, public)
		}
	})

	for name, test := range map[string]struct {
		runErr error
		stderr string
		code   string
	}{
		"exact missing":    {runErr: errors.New("inspect failed"), stderr: "Error: No such image: example.invalid/runtime:test\n", code: "image_not_found"},
		"daemon failure":   {runErr: errors.New("daemon unavailable"), stderr: "Cannot connect to Docker daemon\n", code: "runtime_image_unavailable"},
		"internal timeout": {runErr: context.DeadlineExceeded, code: "runtime_image_unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			runner := &runtimeCompatibilityFailureRunner{err: test.runErr, diagnostic: test.stderr}
			runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			err = runtime.validateCompatibleImage(context.Background(), "example.invalid/runtime:test")
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code {
				t.Fatalf("classification = %v / %+v, want %q", err, public, test.code)
			}
		})
	}

	t.Run("cancellation preserved", func(t *testing.T) {
		runner := &runtimeCompatibilityFailureRunner{err: context.Canceled}
		runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := runtime.validateCompatibleImage(ctx, "example.invalid/runtime:test"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation = %v", err)
		}
	})
}

type runtimeCompatibilityFailureRunner struct {
	err        error
	diagnostic string
}

func (r *runtimeCompatibilityFailureRunner) Run(_ context.Context, _ []string, _ []string, _ io.Reader, _ io.Writer, stderr io.Writer) error {
	_, _ = io.WriteString(stderr, r.diagnostic)
	return r.err
}

func (*runtimeCompatibilityFailureRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("compatibility observation must use bounded Run")
}

func (r *managedRuntimeCompatibilityRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, _ io.Writer) error {
	r.args = append([]string{}, args...)
	_, err := io.WriteString(stdout, r.inspection)
	return err
}

func (*managedRuntimeCompatibilityRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("managed Runtime compatibility must use bounded output")
}

func TestManagedRuntimeCompatibilityRejectsDeclaredVolumes(t *testing.T) {
	root := t.TempDir()
	compatible := `{"id":"sha256:` + strings.Repeat("a", 64) + `","api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`
	for name, inspection := range map[string]string{
		"null volume declaration":   compatible + `,"volumes":null}`,
		"empty volume declaration":  compatible + `,"volumes":{}}`,
		"anonymous writable volume": compatible + `,"volumes":{"/data":{}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			runner := &managedRuntimeCompatibilityRunner{inspection: inspection}
			runtime, err := newRuntime(
				filepath.Join(root, "config"), filepath.Join(root, "state"), runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			err = runtime.validateManagedRuntimeBuildCompatibility(context.Background(), "sha256:"+strings.Repeat("a", 64))
			if name == "anonymous writable volume" {
				if err == nil {
					t.Fatal("managed Runtime with a declared writable volume passed compatibility")
				}
			} else if err != nil {
				t.Fatalf("volume-free managed Runtime compatibility: %v", err)
			}
			if len(runner.args) < 4 || !strings.Contains(runner.args[3], ".Config.Volumes") {
				t.Fatalf("managed Runtime compatibility omitted exact volume metadata: %v", runner.args)
			}
		})
	}
}
