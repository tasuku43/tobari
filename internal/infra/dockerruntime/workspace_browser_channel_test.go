package dockerruntime

import (
	"bufio"
	"context"
	"encoding/json"
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
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	channel := &workspaceBrowserChannel{requestIn: requestReader, response: responseWriter}
	go channel.serve(bridge)
	defer channel.close()

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
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	channel := &workspaceBrowserChannel{requestIn: requestReader, response: responseWriter}
	go channel.serve(bridge)
	defer channel.close()
	reader := bufio.NewReader(responseReader)

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
	bridge := newWorkspaceLoginBridge(
		context.Background(),
		&Runtime{runner: &codexBridgeRunner{projectID: projectID}, browser: browser},
		container, projectID,
	)
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
	mu   sync.Mutex
	args []string
	done chan struct{}
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
	ctx context.Context, args, _ []string, _ io.Reader, _ io.Writer, _ io.Writer,
) error {
	r.mu.Lock()
	r.args = append([]string(nil), args...)
	r.mu.Unlock()
	close(r.done)
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkspaceBrowserChannelUsesSeparateNonTTYDockerExec(t *testing.T) {
	runner := &recordingWorkspaceBrowserControlRunner{done: make(chan struct{})}
	bridge := &workspaceLoginBridge{}
	channel, err := (&Runtime{runner: runner}).startWorkspaceBrowserChannel(context.Background(), bridge, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.done:
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
	forwarded, err := bufio.NewReader(stdout).ReadString('\n')
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
