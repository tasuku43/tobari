package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	hostGitIdentityTimeout        = 2 * time.Second
	maxHostGitIdentityOutputBytes = tobari.MaxContextGitIdentityValueBytes + 1 // value plus Git's NUL delimiter
	maxHostGitDiagnosticBytes     = 4 * 1024
)

type projectGitIdentity struct {
	Name  string
	Email string
}

type hostGitIdentityResolver interface {
	Resolve(context.Context, string) (*projectGitIdentity, error)
}

type hostGitCommandRunner interface {
	Run(context.Context, string, []string, []string, io.Writer, io.Writer) error
}

type osHostGitCommandRunner struct{}

func (osHostGitCommandRunner) Run(
	ctx context.Context,
	executable string,
	args []string,
	environment []string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- executable is resolved outside the project and argv is fixed below.
	command.Env = environment
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

type osHostGitIdentityResolver struct {
	lookPath    func(string) (string, error)
	environment func() []string
	runner      hostGitCommandRunner
}

func newOSHostGitIdentityResolver() hostGitIdentityResolver {
	return &osHostGitIdentityResolver{
		lookPath:    exec.LookPath,
		environment: os.Environ,
		runner:      osHostGitCommandRunner{},
	}
}

func (r *osHostGitIdentityResolver) Resolve(
	ctx context.Context, root string,
) (*projectGitIdentity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
		return nil, gitIdentityResolutionFailed()
	}
	executable, err := r.resolveExecutable(root)
	if err != nil {
		return nil, gitIdentityResolutionFailed()
	}
	environment, err := hostGitEnvironment(r.environment(), root)
	if err != nil {
		return nil, gitIdentityResolutionFailed()
	}
	name, namePresent, err := r.readExactKey(ctx, executable, root, environment, "user.name")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, gitIdentityResolutionFailed()
	}
	email, emailPresent, err := r.readExactKey(ctx, executable, root, environment, "user.email")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, gitIdentityResolutionFailed()
	}
	if !namePresent || !emailPresent {
		return nil, nil
	}
	return &projectGitIdentity{Name: name, Email: email}, nil
}

func (r *osHostGitIdentityResolver) resolveExecutable(root string) (string, error) {
	if r == nil || r.lookPath == nil || r.runner == nil || r.environment == nil {
		return "", fmt.Errorf("host Git resolver is incomplete")
	}
	candidate, err := r.lookPath("git")
	if err != nil || candidate == "" {
		return "", fmt.Errorf("resolve Git executable")
	}
	if !filepath.IsAbs(candidate) {
		candidate, err = filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("make Git executable absolute")
		}
	}
	candidate = filepath.Clean(candidate)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve Git executable symlinks")
	}
	resolved = filepath.Clean(resolved)
	if !filepath.IsAbs(resolved) || isPathAncestor(root, candidate) || isPathAncestor(root, resolved) {
		return "", fmt.Errorf("Git executable is not outside the project root")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("Git executable is unsafe")
	}
	return resolved, nil
}

func (r *osHostGitIdentityResolver) readExactKey(
	ctx context.Context,
	executable string,
	root string,
	environment []string,
	key string,
) (string, bool, error) {
	callContext, cancel := context.WithTimeout(ctx, hostGitIdentityTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: maxHostGitIdentityOutputBytes}
	stderr := &boundedBuffer{limit: maxHostGitDiagnosticBytes}
	err := r.runner.Run(
		callContext,
		executable,
		[]string{"-C", root, "config", "--global", "--includes", "--null", "--get", key},
		environment,
		stdout,
		stderr,
	)
	if stdout.overflow || stderr.overflow {
		return "", false, fmt.Errorf("Git output exceeded its bound")
	}
	data := stdout.buffer.Bytes()
	if err != nil {
		var exited interface{ ExitCode() int }
		if errors.As(err, &exited) && exited.ExitCode() == 1 && len(data) == 0 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("Git config lookup failed")
	}
	value, err := parseHostGitIdentityValue(data)
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func hostGitEnvironment(environment []string, root string) ([]string, error) {
	values := make(map[string]string, 2)
	for _, entry := range environment {
		name, value, present := strings.Cut(entry, "=")
		if !present || (name != "HOME" && name != "XDG_CONFIG_HOME") {
			continue
		}
		if _, duplicate := values[name]; duplicate {
			return nil, fmt.Errorf("duplicate host Git environment path")
		}
		if err := validateHostGitConfigDirectory(root, value); err != nil {
			return nil, fmt.Errorf("unsafe host Git environment path")
		}
		values[name] = value
	}
	home, ok := values["HOME"]
	if !ok {
		return nil, fmt.Errorf("host Git HOME is unavailable")
	}
	result := []string{"HOME=" + home}
	if xdg, ok := values["XDG_CONFIG_HOME"]; ok {
		result = append(result, "XDG_CONFIG_HOME="+xdg)
	}
	return append(result,
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
	), nil
}

func validateHostGitConfigDirectory(root, directory string) error {
	if directory == "" || strings.IndexByte(directory, 0) >= 0 ||
		!filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return fmt.Errorf("path is not a clean absolute path")
	}
	resolved, err := canonicalPathWithMissing(directory)
	if err != nil {
		return err
	}
	resolvedRoot, err := canonicalPathWithMissing(root)
	if err != nil {
		return err
	}
	if isPathAncestor(root, directory) || isPathAncestor(resolvedRoot, resolved) {
		return fmt.Errorf("path is controlled by the project root")
	}
	return nil
}

func parseHostGitIdentityValue(data []byte) (string, error) {
	if len(data) < 2 || len(data) > maxHostGitIdentityOutputBytes || data[len(data)-1] != 0 ||
		strings.Count(string(data), "\x00") != 1 {
		return "", fmt.Errorf("Git identity output is not one NUL-terminated value")
	}
	value := string(data[:len(data)-1])
	if err := validateProjectGitIdentityValue(value); err != nil {
		return "", err
	}
	return value, nil
}

func validateProjectGitIdentityValue(value string) error {
	return tobari.ValidateContextGitIdentityValue(value)
}

func gitIdentityResolutionFailed() *fault.Error {
	return fault.New(
		fault.KindUnavailable,
		"git_identity_resolution_failed",
		"The inherited Git identity could not be resolved safely.",
		false,
		fault.NextAction{
			Command: "manifest show",
			Reason:  "Inspect the selected Context without changing Workspace state.",
		},
	)
}
