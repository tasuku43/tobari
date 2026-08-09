package companionruntime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

type bridgeCommand struct {
	Path string
	Args []string
	Env  []string
}

type bridgeRunner interface {
	Run(context.Context, bridgeCommand, func(io.Reader, io.Writer) error) error
}

type refreshDriver interface {
	Refresh(
		context.Context, credentialhost.State,
	) (credentialhost.TemporaryCredentials, credentialhost.State, error)
}

type execBridgeRunner struct{}

const companionBridgeWaitDelay = 2 * time.Second

func (execBridgeRunner) Run(
	ctx context.Context,
	request bridgeCommand,
	session func(io.Reader, io.Writer) error,
) error {
	if ctx == nil || request.Path != "docker" || session == nil {
		return ErrUnavailable
	}
	processContext, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(processContext, request.Path, request.Args...) // #nosec G204 -- Run supplies fixed Docker argv bound to a validated container ID.
	command.Env = append([]string(nil), request.Env...)
	command.Stderr = io.Discard
	command.WaitDelay = companionBridgeWaitDelay
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("%w: open bridge stdin", ErrUnavailable)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("%w: open bridge stdout", ErrUnavailable)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("%w: start Docker bridge", ErrUnavailable)
	}
	sessionResult := make(chan error, 1)
	go func() { sessionResult <- session(stdout, stdin) }()
	var sessionErr error
	select {
	case sessionErr = <-sessionResult:
		cancel()
	case <-ctx.Done():
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		sessionErr = <-sessionResult
	}
	_ = stdin.Close()
	_ = stdout.Close()
	waitErr := command.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if sessionErr != nil {
		return sessionErr
	}
	if waitErr != nil {
		return fmt.Errorf("%w: Docker bridge stopped", ErrUnavailable)
	}
	return nil
}

// Run consumes one exact private bootstrap and serves until the broker bridge
// closes or the caller cancels. It intentionally emits no user-facing text.
func Run(ctx context.Context, input io.Reader) error {
	return run(ctx, input, execBridgeRunner{}, credentialhost.NewDriver(nil))
}

func run(ctx context.Context, input io.Reader, runner bridgeRunner, driver refreshDriver) error {
	if ctx == nil || runner == nil || driver == nil {
		return ErrUnavailable
	}
	bootstrap, err := decodeBootstrap(input)
	if err != nil {
		return err
	}
	defer bootstrap.Clear()
	if err := prepareCompanionDirectory(bootstrap.document.StateDirectory); err != nil {
		return err
	}
	lock, err := tryAcquireLifetimeLock(lifetimeLockPath(bootstrap.document.StateDirectory))
	if err != nil {
		return err
	}
	defer lock.Close()
	user := strconv.Itoa(bootstrap.document.UID) + ":" + strconv.Itoa(bootstrap.document.GID)
	request := bridgeCommand{
		Path: "docker",
		Args: []string{
			"exec", "-i", "--user", user, bootstrap.document.ContainerID,
			"python3", "-m", "authbroker.companion_bridge",
		},
		Env: dockerEnvironment(os.LookupEnv),
	}
	return runner.Run(ctx, request, func(source io.Reader, destination io.Writer) error {
		return serveSession(ctx, source, destination, bootstrap, driver)
	})
}
