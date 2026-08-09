package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

type hostGitTestResult struct {
	stdout []byte
	stderr []byte
	err    error
}

type hostGitTestCall struct {
	executable  string
	args        []string
	environment []string
	deadline    time.Time
}

type hostGitTestRunner struct {
	results []hostGitTestResult
	calls   []hostGitTestCall
}

func (r *hostGitTestRunner) Run(
	ctx context.Context,
	executable string,
	args []string,
	environment []string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	deadline, _ := ctx.Deadline()
	r.calls = append(r.calls, hostGitTestCall{
		executable: executable, args: append([]string(nil), args...),
		environment: append([]string(nil), environment...), deadline: deadline,
	})
	index := len(r.calls) - 1
	if index >= len(r.results) {
		return errors.New("unexpected host Git call")
	}
	result := r.results[index]
	if len(result.stdout) != 0 {
		if _, err := stdout.Write(result.stdout); err != nil {
			return err
		}
	}
	if len(result.stderr) != 0 {
		if _, err := stderr.Write(result.stderr); err != nil {
			return err
		}
	}
	return result.err
}

type hostGitExitError int

func (e hostGitExitError) Error() string { return "synthetic Git exit" }
func (e hostGitExitError) ExitCode() int { return int(e) }

func hostGitTestExecutable(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "git")
	if err := os.WriteFile(path, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func canonicalTestDirectory(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(root)
}

func TestHostGitIdentityResolverUsesTwoFixedBoundedCalls(t *testing.T) {
	t.Parallel()
	root := canonicalTestDirectory(t)
	executable := hostGitTestExecutable(t, canonicalTestDirectory(t))
	runner := &hostGitTestRunner{results: []hostGitTestResult{
		{stdout: []byte("Tobari User\x00")},
		{stdout: []byte("tobari@example.com\x00")},
	}}
	resolver := &osHostGitIdentityResolver{
		lookPath: func(name string) (string, error) {
			if name != "git" {
				t.Fatalf("lookPath(%q), want git", name)
			}
			return executable, nil
		},
		environment: func() []string {
			return []string{
				"PATH=.", "HOME=/synthetic/home", "XDG_CONFIG_HOME=/synthetic/config",
				"LD_PRELOAD=./repository-library.so", "LD_LIBRARY_PATH=.",
				"DYLD_INSERT_LIBRARIES=./repository-library.dylib", "BASH_ENV=./repository-shell",
				"ENV=./repository-shell", "PROMPT_COMMAND=repository-command", "LC_ALL=host-locale",
				"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.email", "GIT_CONFIG_VALUE_0=leak@example.com",
				"GIT_DIR=/untrusted", "GIT_TRACE=1", "GIT_PAGER=untrusted",
			}
		},
		runner: runner,
	}
	started := time.Now()
	identity, err := resolver.Resolve(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if identity == nil || identity.Name != "Tobari User" || identity.Email != "tobari@example.com" {
		t.Fatalf("identity = %#v", identity)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("host Git calls = %d, want 2", len(runner.calls))
	}
	for index, key := range []string{"user.name", "user.email"} {
		call := runner.calls[index]
		if call.executable != executable {
			t.Errorf("call %d executable = %q, want %q", index, call.executable, executable)
		}
		wantArgs := []string{"-C", root, "config", "--global", "--includes", "--null", "--get", key}
		if !slices.Equal(call.args, wantArgs) {
			t.Errorf("call %d args = %v, want %v", index, call.args, wantArgs)
		}
		remaining := time.Until(call.deadline)
		if call.deadline.Before(started) || remaining <= 0 || remaining > hostGitIdentityTimeout+time.Second {
			t.Errorf("call %d deadline is not bounded to %s: %v", index, hostGitIdentityTimeout, call.deadline)
		}
		wantEnvironment := []string{
			"HOME=/synthetic/home", "XDG_CONFIG_HOME=/synthetic/config", "LC_ALL=C",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat",
		}
		if !slices.Equal(call.environment, wantEnvironment) {
			t.Errorf("call %d environment = %v, want exact allowlist %v", index, call.environment, wantEnvironment)
		}
	}
}

func TestHostGitIdentityResolverUsesRootConditionalGlobalValues(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	base := canonicalTestDirectory(t)
	home := filepath.Join(base, "home")
	root := filepath.Join(base, "repository")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	initCommand := exec.Command(git, "init", "--quiet", root) // #nosec G204 -- test invokes resolved Git with synthetic fixed arguments.
	initCommand.Env = []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "PATH=" + os.Getenv("PATH")}
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("initialize synthetic repository: %v: %s", err, output)
	}
	repositorySelected := filepath.Join(base, "repository-selected.gitconfig")
	if err := os.WriteFile(repositorySelected, []byte("[user]\n\tname = Repository Selected\n\temail = repository@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localIncludeCommand := exec.Command(git, "-C", root, "config", "include.path", repositorySelected) // #nosec G204 -- test invokes resolved Git with synthetic fixed arguments.
	localIncludeCommand.Env = []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "PATH=" + os.Getenv("PATH")}
	if output, err := localIncludeCommand.CombinedOutput(); err != nil {
		t.Fatalf("configure repository-selected include: %v: %s", err, output)
	}
	conditional := filepath.Join(base, "conditional.gitconfig")
	if err := os.WriteFile(conditional, []byte("[user]\n\tname = Conditional User\n\temail = conditional@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	global := fmt.Sprintf(
		"[user]\n\tname = Global User\n\temail = global@example.com\n[includeIf \"gitdir:%s/\"]\n\tpath = %s\n",
		root, conditional,
	)
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(global), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &osHostGitIdentityResolver{
		lookPath: func(string) (string, error) { return git, nil },
		environment: func() []string {
			return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LC_ALL=C"}
		},
		runner: osHostGitCommandRunner{},
	}
	identity, err := resolver.Resolve(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if identity == nil || identity.Name != "Conditional User" || identity.Email != "conditional@example.com" {
		t.Fatalf("root-conditional identity = %#v", identity)
	}
}

func TestHostGitIdentityResolverTreatsIncompletePairAsNoFallback(t *testing.T) {
	t.Parallel()
	runner := &hostGitTestRunner{results: []hostGitTestResult{
		{stderr: []byte("synthetic bounded absence diagnostic"), err: hostGitExitError(1)},
		{stdout: []byte("tobari@example.com\x00")},
	}}
	resolver := &osHostGitIdentityResolver{
		lookPath:    func(string) (string, error) { return hostGitTestExecutable(t, canonicalTestDirectory(t)), nil },
		environment: func() []string { return []string{"HOME=/synthetic/home", "PATH=/synthetic"} },
		runner:      runner,
	}
	identity, err := resolver.Resolve(context.Background(), canonicalTestDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	if identity != nil {
		t.Fatalf("incomplete identity = %#v, want no fallback", identity)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("host Git calls = %d, want both exact keys", len(runner.calls))
	}
}

func TestHostGitIdentityResolverCollapsesExecutionDiagnostics(t *testing.T) {
	t.Parallel()
	const privateDiagnostic = "Private User <private@example.com> from /private/config"
	runner := &hostGitTestRunner{results: []hostGitTestResult{{
		stderr: []byte(privateDiagnostic), err: hostGitExitError(2),
	}}}
	resolver := &osHostGitIdentityResolver{
		lookPath:    func(string) (string, error) { return hostGitTestExecutable(t, canonicalTestDirectory(t)), nil },
		environment: func() []string { return []string{"HOME=/synthetic/home"} },
		runner:      runner,
	}
	_, err := resolver.Resolve(context.Background(), canonicalTestDirectory(t))
	assertGitIdentityResolutionFault(t, err)
	if strings.Contains(err.Error(), "Private") || strings.Contains(err.Error(), "private@") || strings.Contains(err.Error(), "/private") {
		t.Fatalf("public fault leaked raw Git diagnostic: %q", err)
	}
}

func TestHostGitIdentityResolverRejectsUnsafeOutput(t *testing.T) {
	t.Parallel()
	oversized := append(bytesOf('a', tobariGitIdentityMaxForTest()+1), 0)
	for name, output := range map[string][]byte{
		"missing delimiter": []byte("Tobari User"),
		"multiple values":   []byte("First\x00Second\x00"),
		"empty":             {0},
		"control":           []byte("Tobari\nUser\x00"),
		"format":            []byte("Tobari\u200dUser\x00"),
		"invalid UTF-8":     {0xff, 0},
		"oversized":         oversized,
	} {
		name, output := name, output
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := &hostGitTestRunner{results: []hostGitTestResult{{stdout: output}}}
			resolver := &osHostGitIdentityResolver{
				lookPath:    func(string) (string, error) { return hostGitTestExecutable(t, canonicalTestDirectory(t)), nil },
				environment: func() []string { return []string{"HOME=/synthetic/home"} },
				runner:      runner,
			}
			_, err := resolver.Resolve(context.Background(), canonicalTestDirectory(t))
			assertGitIdentityResolutionFault(t, err)
		})
	}
}

func TestHostGitIdentityResolverRejectsProjectSelectedExecutable(t *testing.T) {
	t.Parallel()
	root := canonicalTestDirectory(t)
	executable := hostGitTestExecutable(t, root)
	runner := &hostGitTestRunner{}
	resolver := &osHostGitIdentityResolver{
		lookPath:    func(string) (string, error) { return executable, nil },
		environment: func() []string { return []string{} },
		runner:      runner,
	}
	_, err := resolver.Resolve(context.Background(), root)
	assertGitIdentityResolutionFault(t, err)
	if len(runner.calls) != 0 {
		t.Fatalf("project-selected fake Git was executed: %v", runner.calls)
	}
}

func TestHostGitIdentityResolverRejectsProjectSelectedConfigDirectories(t *testing.T) {
	t.Parallel()
	root := canonicalTestDirectory(t)
	executable := hostGitTestExecutable(t, canonicalTestDirectory(t))
	outside := canonicalTestDirectory(t)
	linkedIntoRoot := filepath.Join(outside, "linked-config")
	if err := os.Symlink(root, linkedIntoRoot); err != nil {
		t.Fatal(err)
	}
	for name, environment := range map[string][]string{
		"missing HOME":     {"PATH=/synthetic"},
		"HOME inside root": {"HOME=" + root},
		"XDG inside root":  {"HOME=/synthetic/home", "XDG_CONFIG_HOME=" + filepath.Join(root, "config")},
		"XDG resolves into root": {
			"HOME=/synthetic/home", "XDG_CONFIG_HOME=" + linkedIntoRoot,
		},
		"duplicate HOME": {"HOME=/synthetic/home", "HOME=/other/home"},
	} {
		name, environment := name, environment
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := &hostGitTestRunner{}
			resolver := &osHostGitIdentityResolver{
				lookPath: func(string) (string, error) { return executable, nil },
				environment: func() []string {
					return append([]string(nil), environment...)
				},
				runner: runner,
			}
			_, err := resolver.Resolve(context.Background(), root)
			assertGitIdentityResolutionFault(t, err)
			if len(runner.calls) != 0 {
				t.Fatalf("unsafe config directory reached Git: %v", runner.calls)
			}
		})
	}
}

func TestHostGitIdentityResolverHonorsCallerCancellationBeforeExecution(t *testing.T) {
	t.Parallel()
	runner := &hostGitTestRunner{}
	resolver := &osHostGitIdentityResolver{
		lookPath:    func(string) (string, error) { return hostGitTestExecutable(t, canonicalTestDirectory(t)), nil },
		environment: func() []string { return []string{} },
		runner:      runner,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolver.Resolve(ctx, canonicalTestDirectory(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("canceled resolution executed Git: %v", runner.calls)
	}
}

func assertGitIdentityResolutionFault(t *testing.T, err error) {
	t.Helper()
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Code != "git_identity_resolution_failed" || structured.Retryable {
		t.Fatalf("error = %#v, want non-retryable git_identity_resolution_failed", err)
	}
	if len(structured.NextActions) != 1 || structured.NextActions[0].Command != "context show" {
		t.Fatalf("next actions = %#v, want context show", structured.NextActions)
	}
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}

func tobariGitIdentityMaxForTest() int {
	return maxHostGitIdentityOutputBytes - 1
}
