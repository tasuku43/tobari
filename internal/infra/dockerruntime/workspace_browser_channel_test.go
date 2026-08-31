package dockerruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestWorkspaceBrowserRequestDecoderAcceptsOnlyExactSchema(t *testing.T) {
	valid := `{"schema_version":1,"target":"` + syntheticTWGVerificationURL + `"}`
	request, ok := decodeWorkspaceBrowserRequest([]byte(valid))
	if !ok || request.SchemaVersion != 1 || request.Target != syntheticTWGVerificationURL {
		t.Fatalf("valid request = (%+v, %t)", request, ok)
	}
	for _, hostile := range []string{
		``,
		`[]`,
		`{"schema_version":2,"target":"` + syntheticTWGVerificationURL + `"}`,
		`{"schema_version":1.0,"target":"` + syntheticTWGVerificationURL + `"}`,
		`{"schema_version":1,"schema_version":1,"target":"` + syntheticTWGVerificationURL + `"}`,
		`{"schema_version":1,"target":"` + syntheticTWGVerificationURL + `","target":"` + githubDeviceURL + `"}`,
		`{"schema_version":1,"target":"` + syntheticTWGVerificationURL + `","extra":true}`,
		`{"schema_version":1,"target":null}`,
		`{"schema_version":1,"target":""}`,
		valid + `{}`,
		`{"schema_version":1,"target":"` + strings.Repeat("x", workspaceLoginLineLimit+1) + `"}`,
	} {
		if decoded, accepted := decodeWorkspaceBrowserRequest([]byte(hostile)); accepted {
			t.Fatalf("hostile request accepted: %+v from %q", decoded, hostile)
		}
	}
}

func TestWorkspaceBrowserChannelOpensValidatedTargetAndReturnsExactResponse(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge, err := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	if err != nil {
		t.Fatal(err)
	}
	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	channel := &workspaceBrowserChannel{requestIn: requestReader, response: responseWriter}
	go func() { _ = channel.serve(bridge) }()
	defer channel.close()
	_, _ = io.WriteString(requestWriter, workspaceBrowserReadyFrame+"\n")

	for range 2 {
		_, _ = io.WriteString(requestWriter, `{"schema_version":1,"target":"`+syntheticTWGVerificationURL+`"}`+"\n")
		line, err := bufio.NewReader(responseReader).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line != `{"schema_version":1,"ok":true}`+"\n" {
			t.Fatalf("response = %q", line)
		}
	}
	if !reflect.DeepEqual(browser.targets, []string{syntheticTWGVerificationURL}) {
		t.Fatalf("browser targets = %q", browser.targets)
	}
}

func TestWorkspaceBrowserChannelRejectsMalformedAndUnreviewedTargets(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge, err := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	if err != nil {
		t.Fatal(err)
	}
	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	channel := &workspaceBrowserChannel{requestIn: requestReader, response: responseWriter}
	go func() { _ = channel.serve(bridge) }()
	defer channel.close()
	reader := bufio.NewReader(responseReader)
	_, _ = io.WriteString(requestWriter, workspaceBrowserReadyFrame+"\n")

	for _, request := range []string{
		`{"schema_version":1,"target":"https://example.com/"}`,
		`{"schema_version":1,"target":"` + syntheticTWGVerificationURL + `","extra":true}`,
		`not-json`,
	} {
		_, _ = io.WriteString(requestWriter, request+"\n")
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line != `{"schema_version":1,"ok":false}`+"\n" {
			t.Fatalf("response = %q for %q", line, request)
		}
	}
	if len(browser.targets) != 0 {
		t.Fatalf("rejected requests opened browser: %q", browser.targets)
	}
}

func TestWorkspaceBrowserBridgeCapsUniqueHostOpenAttempts(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	browser := &recordingBrowser{}
	bridge, err := newWorkspaceLoginBridge(
		context.Background(),
		&Runtime{runner: &codexBridgeRunner{projectID: projectID}, browser: browser},
		container, projectID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	for index := range workspaceLoginURLBudget + 1 {
		target := fmt.Sprintf("https://auth.atlassian.com/oauth/activate?user_code=CODE-%04d", index)
		opened := bridge.trigger(target)
		if opened != (index < workspaceLoginURLBudget) {
			t.Fatalf("attempt %d success = %t", index, opened)
		}
	}
	if len(browser.targets) != workspaceLoginURLBudget {
		t.Fatalf("browser targets = %d, want %d", len(browser.targets), workspaceLoginURLBudget)
	}
}

type recordingWorkspaceBrowserControlRunner struct {
	mu         sync.Mutex
	args       []string
	started    chan struct{}
	release    <-chan struct{}
	exitBefore error
}

type relayLossWorkspaceBrowserControlRunner struct {
	started chan struct{}
	stopped chan struct{}
	release chan struct{}
}

func (r *relayLossWorkspaceBrowserControlRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	return nil
}

func (r *relayLossWorkspaceBrowserControlRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

func (r *relayLossWorkspaceBrowserControlRunner) RunWorkspaceBrowserControl(
	ctx context.Context, _ []string, _ []string, _ io.Reader, out io.Writer, _ io.Writer,
) error {
	if _, err := io.WriteString(out, workspaceBrowserReadyFrame+"\n"); err != nil {
		return err
	}
	close(r.started)
	<-r.release
	if closer, ok := out.(io.Closer); ok {
		_ = closer.Close()
	}
	<-ctx.Done()
	close(r.stopped)
	return ctx.Err()
}

func (r *recordingWorkspaceBrowserControlRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	return nil
}

func (r *recordingWorkspaceBrowserControlRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

func (r *recordingWorkspaceBrowserControlRunner) RunWorkspaceBrowserControl(
	ctx context.Context, args, _ []string, _ io.Reader, out io.Writer, _ io.Writer,
) error {
	r.mu.Lock()
	r.args = append([]string(nil), args...)
	r.mu.Unlock()
	close(r.started)
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if r.exitBefore != nil {
		return r.exitBefore
	}
	_, _ = io.WriteString(out, `{"schema_version":1,"ready":true}`+"\n")
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkspaceBrowserChannelUsesSeparateNonTTYDockerExec(t *testing.T) {
	runner := &recordingWorkspaceBrowserControlRunner{started: make(chan struct{})}
	bridge := &workspaceLoginBridge{}
	channel, err := (&Runtime{runner: runner}).startWorkspaceBrowserChannel(context.Background(), bridge, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("browser control runner did not start")
	}
	channel.close()
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !containsArgSequence(runner.args, "exec", "-i", "--user") ||
		!containsArgSequence(runner.args, "workspace", "python3", "-c", workspaceBrowserAgentProgram, channel.socketPath) {
		t.Fatalf("browser control argv = %q", runner.args)
	}
	for _, forbidden := range []string{"-t", "--tty"} {
		if slicesContains(runner.args, forbidden) {
			t.Fatalf("browser control argv contains %q: %q", forbidden, runner.args)
		}
	}
}

func TestWorkspaceBrowserChannelWaitsForAgentReadiness(t *testing.T) {
	release := make(chan struct{})
	runner := &recordingWorkspaceBrowserControlRunner{started: make(chan struct{}), release: release}
	result := make(chan error, 1)
	go func() {
		channel, err := (&Runtime{runner: runner}).startWorkspaceBrowserChannel(
			context.Background(), &workspaceLoginBridge{}, "workspace",
		)
		if channel != nil {
			channel.close()
		}
		result <- err
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("browser control runner did not start")
	}
	select {
	case err := <-result:
		t.Fatalf("channel returned before readiness: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("ready channel failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ready channel did not return")
	}
}

func TestWorkspaceBrowserChannelRejectsControlExitBeforeReadiness(t *testing.T) {
	runner := &recordingWorkspaceBrowserControlRunner{
		started: make(chan struct{}), exitBefore: errors.New("synthetic control failure"),
	}
	channel, err := (&Runtime{runner: runner}).startWorkspaceBrowserChannel(
		context.Background(), &workspaceLoginBridge{}, "workspace",
	)
	if err == nil || channel != nil || !strings.Contains(err.Error(), "before readiness") {
		t.Fatalf("start result = (%+v, %v), want readiness failure", channel, err)
	}
}

func TestWorkspaceBrowserControlExitAfterReadinessCancelsAttachedCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	channel := &workspaceBrowserChannel{result: make(chan error, 1)}
	started := make(chan struct{})
	stopped := make(chan struct{})
	run := func() error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}
	go func() {
		<-started
		channel.result <- errors.New("synthetic post-ready control exit")
	}()
	err := runWithAttachedBrowserControl(context.Background(), cancel, channel, run)
	if !errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
		t.Fatalf("post-ready control exit error=%v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("attached Workspace command was not canceled")
	}
}

func TestWorkspaceBrowserHostRelayLossAfterReadinessCancelsAttachedCommand(t *testing.T) {
	runner := &relayLossWorkspaceBrowserControlRunner{started: make(chan struct{}), stopped: make(chan struct{}), release: make(chan struct{})}
	controlContext, cancelControl := context.WithCancel(context.Background())
	defer cancelControl()
	channel, err := (&Runtime{runner: runner}).startWorkspaceBrowserChannel(controlContext, &workspaceLoginBridge{}, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer channel.close()
	close(runner.release)
	attachedContext, cancelAttached := context.WithCancel(context.Background())
	attachedStopped := make(chan struct{})
	err = runWithAttachedBrowserControl(context.Background(), cancelAttached, channel, func() error {
		<-attachedContext.Done()
		close(attachedStopped)
		return attachedContext.Err()
	})
	if !errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
		t.Fatalf("host relay loss error=%v", err)
	}
	select {
	case <-attachedStopped:
	default:
		t.Fatal("attached Workspace command was not canceled after host relay loss")
	}
	select {
	case <-runner.stopped:
	case <-time.After(time.Second):
		t.Fatal("Docker control process was not canceled after host relay loss")
	}
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestWorkspaceBrowserAgentRelaysOneFramedRequestAndCleansSocket(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required by runtime API 1")
	}
	socketPath := shortTestUnixSocket(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, "-c", workspaceBrowserAgentProgram, socketPath)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stdoutReader := bufio.NewReader(stdout)
	ready, err := stdoutReader.ReadString('\n')
	if err != nil || ready != `{"schema_version":1,"ready":true}`+"\n" {
		t.Fatalf("agent readiness = %q, %v", ready, err)
	}

	var connection net.Conn
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		connection, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect agent socket: %v", err)
	}
	request := `{"schema_version":1,"target":"` + syntheticTWGVerificationURL + `"}` + "\n"
	_, _ = io.WriteString(connection, request)
	forwarded, err := stdoutReader.ReadString('\n')
	if err != nil || forwarded != request {
		t.Fatalf("forwarded request = %q, %v", forwarded, err)
	}
	response := `{"schema_version":1,"ok":true}` + "\n"
	_, _ = io.WriteString(stdin, response)
	got, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil || got != response {
		t.Fatalf("agent response = %q, %v", got, err)
	}
	_ = connection.Close()
	_ = stdin.Close()
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("agent socket remains after close: %v", err)
	}
}

func TestEmbeddedWorkspaceBrowserOpenerUsesOnlySocketProtocol(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "runtime")
	if err := runtimeassets.Materialize(destination); err != nil {
		t.Fatal(err)
	}
	socketPath := shortTestUnixSocket(t)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan workspaceBrowserRequest, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		line, _ := bufio.NewReader(connection).ReadBytes('\n')
		var request workspaceBrowserRequest
		_ = json.Unmarshal(line, &request)
		received <- request
		_, _ = io.WriteString(connection, `{"schema_version":1,"ok":true}`+"\n")
	}()
	opener := filepath.Join(destination, "browser", "tobari-open")
	command := exec.Command(opener, syntheticTWGVerificationURL)
	command.Env = append(os.Environ(), workspaceBrowserSocketEnv+"="+socketPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("opener failed: %v: %s", err, output)
	} else if len(output) != 0 {
		t.Fatalf("opener emitted output: %q", output)
	}
	select {
	case request := <-received:
		if request.SchemaVersion != 1 || request.Target != syntheticTWGVerificationURL {
			t.Fatalf("opener request = %+v", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("opener request was not received")
	}
}

func shortTestUnixSocket(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "tobari-browser-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "browser.sock")
}

func TestWorkspaceBrowserEnvironmentUsesOneOpenerAndUnpredictableSocket(t *testing.T) {
	first, err := newWorkspaceBrowserSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newWorkspaceBrowserSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "/run/tobari-browser-") || !strings.HasSuffix(first, ".sock") {
		t.Fatalf("socket paths = %q, %q", first, second)
	}
	channel := &workspaceBrowserChannel{socketPath: first}
	want := []string{
		"BROWSER=" + workspaceBrowserOpenerPath,
		"GH_BROWSER=" + workspaceBrowserOpenerPath,
		workspaceBrowserSocketEnv + "=" + first,
	}
	if got := channel.environment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %q, want %q", got, want)
	}
}

func TestWorkspaceBrowserAgentProgramContainsNoGenericHostAuthority(t *testing.T) {
	for _, forbidden := range []string{"docker", "http://", "https://", "subprocess", "os.system", "exec("} {
		if strings.Contains(workspaceBrowserAgentProgram, forbidden) {
			t.Fatalf("agent program contains forbidden authority %q", forbidden)
		}
	}
	if got := fmt.Sprint(workspaceBrowserMessageLimit); got == "" {
		t.Fatal("message limit is not declared")
	}
}
