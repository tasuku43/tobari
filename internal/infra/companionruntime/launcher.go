package companionruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// PrivateArg0 selects the non-public companion entrypoint. A valid, exact
// stdin bootstrap remains mandatory; this value alone grants no operation.
const PrivateArg0 = "tobari-credential-companion"

// Process is a just-started companion. The cluster operation either aborts it
// before readiness or detaches it after broker-confirmed readiness.
type Process interface {
	Abort() error
	Detach() error
}

// Launcher starts the current executable in its private companion mode.
type Launcher interface {
	Start(*Bootstrap) (Process, error)
	WaitForStopped(context.Context, string) error
}

// OSLauncher is the production self-process launcher.
type OSLauncher struct {
	executable func() (string, error)
}

func NewOSLauncher() *OSLauncher {
	return &OSLauncher{executable: os.Executable}
}

func (l *OSLauncher) Start(bootstrap *Bootstrap) (Process, error) {
	if l == nil || l.executable == nil || bootstrap == nil {
		return nil, ErrInvalidBootstrap
	}
	encoded, err := bootstrap.Encode()
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	executable, err := l.executable()
	if err != nil || executable == "" {
		return nil, fmt.Errorf("%w: resolve companion executable", ErrUnavailable)
	}
	command := exec.Command(executable) // #nosec G204 -- os.Executable selects the current reviewed Tobari binary.
	command.Args = []string{PrivateArg0}
	command.Env = dockerEnvironment(os.LookupEnv)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	configureDetachedProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: prepare companion bootstrap", ErrUnavailable)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("%w: start companion", ErrUnavailable)
	}
	process := &osStartedProcess{command: command}
	if err := writeBootstrap(stdin, encoded); err != nil {
		_ = process.Abort()
		return nil, err
	}
	return process, nil
}

var dockerEnvironmentNames = [...]string{
	"PATH",
	"HOME",
	"DOCKER_HOST",
	"DOCKER_CONTEXT",
	"DOCKER_CONFIG",
	"DOCKER_TLS_VERIFY",
	"DOCKER_CERT_PATH",
	"DOCKER_API_VERSION",
	"DOCKER_DEFAULT_PLATFORM",
}

func dockerEnvironment(lookup func(string) (string, bool)) []string {
	if lookup == nil {
		return nil
	}
	result := make([]string, 0, len(dockerEnvironmentNames))
	for _, name := range dockerEnvironmentNames {
		if value, ok := lookup(name); ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func writeBootstrap(destination io.WriteCloser, encoded []byte) error {
	defer destination.Close()
	for len(encoded) != 0 {
		written, err := destination.Write(encoded)
		if err != nil || written <= 0 {
			return fmt.Errorf("%w: deliver companion bootstrap", ErrUnavailable)
		}
		encoded = encoded[written:]
	}
	return nil
}

func (*OSLauncher) WaitForStopped(ctx context.Context, stateDirectory string) error {
	if ctx == nil {
		return ErrUnavailable
	}
	return WaitForStopped(ctx, stateDirectory)
}

type osStartedProcess struct {
	mu      sync.Mutex
	command *exec.Cmd
	done    bool
}

func (p *osStartedProcess) Abort() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done || p.command == nil || p.command.Process == nil {
		return nil
	}
	killErr := p.command.Process.Kill()
	waitErr := p.command.Wait()
	p.done = true
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return fmt.Errorf("%w: terminate companion", ErrUnavailable)
	}
	var exit *exec.ExitError
	if waitErr == nil || errors.As(waitErr, &exit) {
		return nil
	}
	return fmt.Errorf("%w: reap companion", ErrUnavailable)
}

func (p *osStartedProcess) Detach() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done || p.command == nil || p.command.Process == nil {
		return nil
	}
	if err := p.command.Process.Release(); err != nil {
		return fmt.Errorf("%w: detach companion", ErrUnavailable)
	}
	p.done = true
	return nil
}
