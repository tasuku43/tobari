package credentialhost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const claudeTokenCanary = "sk-ant-oat01-synthetic_token_canary_1234567890"

type fakeClaudeRunner struct {
	mu    sync.Mutex
	calls int
	run   func(int, context.Context, ClaudeCommand) error
}

func (r *fakeClaudeRunner) Run(ctx context.Context, command ClaudeCommand) error {
	r.mu.Lock()
	call := r.calls
	r.calls++
	r.mu.Unlock()
	if r.run == nil {
		return nil
	}
	return r.run(call, ctx, command)
}

func (r *fakeClaudeRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type claudeExitCodeError int

func (e claudeExitCodeError) Error() string { return "synthetic process exit" }
func (e claudeExitCodeError) ExitCode() int { return int(e) }

func TestClaudeLoginUsesCanonicalExecutableExactVersionSetupTokenAndEnvironment(t *testing.T) {
	target := testClaudeExecutable(t)
	link := filepath.Join(t.TempDir(), "claude-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"ANTHROPIC_API_KEY":                       claudeTokenCanary,
		"ANTHROPIC_AUTH_TOKEN":                    claudeTokenCanary,
		"CLAUDE_CODE_OAUTH_TOKEN":                 claudeTokenCanary,
		"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR": "9",
		"CLAUDE_CONFIG_DIR":                       "/ambient/claude",
		"CLAUDE_CODE_USE_BEDROCK":                 "1",
		"CLAUDE_CODE_USE_VERTEX":                  "1",
		"CLAUDE_CODE_USE_FOUNDRY":                 "1",
		"ANTHROPIC_BASE_URL":                      "https://ambient.example",
		"HTTP_PROXY":                              "http://proxy.example",
		"HTTPS_PROXY":                             "http://proxy.example",
		"ALL_PROXY":                               "http://proxy.example",
		"NO_PROXY":                                "ambient.example",
		"BROWSER":                                 "/ambient/browser",
		"CLAUDE_CODE_REMOTE":                      "1",
		"LD_PRELOAD":                              "/ambient/inject.so",
		"DYLD_INSERT_LIBRARIES":                   "/ambient/inject.dylib",
		"HOME":                                    "/ambient/home",
		"PATH":                                    "/ambient/bin",
	} {
		t.Setenv(name, value)
	}

	stdin := strings.NewReader("trusted terminal input")
	var visible bytes.Buffer
	var home string
	var configDirectory string
	runner := &fakeClaudeRunner{run: func(call int, _ context.Context, command ClaudeCommand) error {
		if command.Path != canonical {
			t.Fatalf("command path = %q, want canonical %q", command.Path, canonical)
		}
		if call == 0 {
			home = command.Dir
			configDirectory = environmentValue(t, command.Env, "CLAUDE_CONFIG_DIR")
			assertPrivatePath(t, home, 0o700)
			assertPrivatePath(t, configDirectory, 0o700)
			if filepath.Dir(configDirectory) != home {
				t.Fatalf("CLAUDE_CONFIG_DIR = %q, want child of %q", configDirectory, home)
			}
		}
		wantEnvironment := []string{
			"HOME=" + home,
			"CLAUDE_CONFIG_DIR=" + configDirectory,
			"NO_COLOR=1",
			"LC_ALL=C",
			"DISABLE_AUTOUPDATER=1",
			"PATH=/usr/bin:/bin",
		}
		if command.Dir != home || !reflect.DeepEqual(command.Env, wantEnvironment) {
			t.Fatalf("command dir/env = %q %#v, want %q %#v", command.Dir, command.Env, home, wantEnvironment)
		}
		switch call {
		case 0:
			if !reflect.DeepEqual(command.Args, []string{"--version"}) || command.Terminal ||
				command.Stdin != nil || command.Stdout == nil || command.Stderr == nil {
				t.Fatalf("version command = %#v", command)
			}
			_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
			return err
		case 1:
			if !reflect.DeepEqual(command.Args, []string{"setup-token"}) || !command.Terminal ||
				command.Stdin != stdin || command.Stdout == nil || command.Stderr != nil {
				t.Fatalf("setup-token command = %#v", command)
			}
			for _, chunk := range successfulClaudeOutputChunks(claudeTokenCanary) {
				if _, err := command.Stdout.Write(chunk); err != nil {
					return err
				}
			}
			return nil
		default:
			t.Fatalf("unexpected command call %d", call)
			return nil
		}
	}}
	driver := NewClaudeDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), link, ClaudeLoginStreams{
		Stdin: stdin, Output: &visible,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 2 || credential.AccountLabel() != ClaudeAccountLabel ||
		credential.DriverID() != ClaudeDriverID || len(credential.DriverRevision()) != 64 ||
		string(credential.Token()) != claudeTokenCanary {
		t.Fatalf("calls=%d credential=%v", runner.callCount(), credential)
	}
	if strings.Contains(visible.String(), claudeTokenCanary) ||
		strings.Contains(visible.String(), claudeSetupTokenMarker) ||
		!strings.Contains(visible.String(), "long-lived (1-year) auth token setup") ||
		strings.ContainsRune(visible.String(), '\x1b') {
		t.Fatalf("visible output was not safely projected: %q", visible.String())
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary Claude HOME was not removed: %v", err)
	}
	returned := credential.Token()
	returned[0] = 'X'
	if string(credential.Token()) != claudeTokenCanary {
		t.Fatal("Token did not return an independent copy")
	}
	for _, rendered := range []string{fmt.Sprintf("%v", credential), fmt.Sprintf("%#v", credential)} {
		if strings.Contains(rendered, claudeTokenCanary) || !strings.Contains(rendered, "redacted") {
			t.Fatalf("credential formatting leaked: %q", rendered)
		}
	}
	credential.Clear()
	if len(credential.Token()) != 0 || credential.AccountLabel() != "" ||
		credential.DriverID() != "" || credential.DriverRevision() != "" {
		t.Fatalf("credential was not empty after Clear: %v", credential)
	}
	var nilCredential *ClaudeCredential
	nilCredential.Clear()
}

func TestClaudeLoginRejectsVersionBeforeSetupToken(t *testing.T) {
	tests := map[string]struct {
		stdout []byte
		stderr []byte
		limit  bool
	}{
		"wrong version":        {stdout: []byte("2.1.219 (Claude Code)\n")},
		"extra stdout framing": {stdout: []byte(claudeVersionOutput + "\n\n")},
		"stderr":               {stdout: []byte(claudeVersionOutput + "\n"), stderr: []byte("warning")},
		"stdout overflow":      {limit: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &fakeClaudeRunner{run: func(call int, _ context.Context, command ClaudeCommand) error {
				if call != 0 {
					t.Fatalf("setup-token ran after invalid version")
				}
				if test.limit {
					_, _ = command.Stdout.Write(bytes.Repeat([]byte("x"), maxClaudeVersionSize+1))
					return nil
				}
				if len(test.stdout) != 0 {
					_, _ = command.Stdout.Write(test.stdout)
				}
				if len(test.stderr) != 0 {
					_, _ = command.Stderr.Write(test.stderr)
				}
				return nil
			}}
			driver := NewClaudeDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(
				context.Background(), testClaudeExecutable(t), claudeTestStreams(),
			)
			want := ErrClaudeVersion
			if test.limit {
				want = ErrClaudeOutputLimit
			}
			if !errors.Is(err, want) || len(credential.Token()) != 0 || runner.callCount() != 1 {
				t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
			}
		})
	}
}

func TestClaudeLoginRejectsExecutableDigestChangeAfterSetupToken(t *testing.T) {
	executable := testClaudeExecutable(t)
	runner := &fakeClaudeRunner{run: func(call int, _ context.Context, command ClaudeCommand) error {
		switch call {
		case 0:
			_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
			return err
		case 1:
			for _, chunk := range successfulClaudeOutputChunks(claudeTokenCanary) {
				if _, err := command.Stdout.Write(chunk); err != nil {
					return err
				}
			}
			return os.WriteFile(executable, []byte("changed synthetic claude executable"), 0o700)
		default:
			t.Fatalf("unexpected command %d", call)
			return nil
		}
	}}
	driver := NewClaudeDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), executable, claudeTestStreams())
	if !errors.Is(err, ErrClaudeExecutable) || len(credential.Token()) != 0 || runner.callCount() != 2 {
		t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
	}
}

func TestClaudeLoginRejectsExecutableDigestChangeAfterVersion(t *testing.T) {
	executable := testClaudeExecutable(t)
	runner := &fakeClaudeRunner{run: func(call int, _ context.Context, command ClaudeCommand) error {
		if call != 0 {
			t.Fatalf("setup-token ran after version executable changed")
		}
		if _, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n")); err != nil {
			return err
		}
		return os.WriteFile(executable, []byte("changed after version"), 0o700)
	}}
	driver := NewClaudeDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), executable, claudeTestStreams())
	if !errors.Is(err, ErrClaudeExecutable) || len(credential.Token()) != 0 || runner.callCount() != 1 {
		t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
	}
}

func TestClaudeLoginCancellationAndRunnerFailureNeverReturnCredentialOrDetails(t *testing.T) {
	tests := map[string]struct {
		setup func(context.Context) error
		want  error
	}{
		"exit 130": {
			setup: func(context.Context) error { return claudeExitCodeError(130) },
			want:  ErrClaudeLoginCancelled,
		},
		"runner failure": {
			setup: func(context.Context) error { return errors.New(claudeTokenCanary + " runner detail") },
			want:  ErrClaudeLoginFailed,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &fakeClaudeRunner{run: func(call int, ctx context.Context, command ClaudeCommand) error {
				if call == 0 {
					_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
					return err
				}
				return test.setup(ctx)
			}}
			driver := NewClaudeDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(
				context.Background(), testClaudeExecutable(t), claudeTestStreams(),
			)
			if !errors.Is(err, test.want) || strings.Contains(err.Error(), claudeTokenCanary) || len(credential.Token()) != 0 {
				t.Fatalf("credential=%v error=%v", credential, err)
			}
		})
	}

	t.Run("context", func(t *testing.T) {
		started := make(chan struct{})
		runner := &fakeClaudeRunner{run: func(call int, ctx context.Context, command ClaudeCommand) error {
			if call == 0 {
				_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
				return err
			}
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}}
		driver := NewClaudeDriver(runner)
		driver.tempRoot = t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := driver.Login(ctx, testClaudeExecutable(t), claudeTestStreams())
			result <- err
		}()
		<-started
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, ErrClaudeLoginCancelled) {
				t.Fatalf("error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Claude login did not honor cancellation")
		}
	})
}

func TestClaudeLoginCleanupFailureSuppressesCredential(t *testing.T) {
	runner := successfulClaudeRunner(t, claudeTokenCanary)
	driver := NewClaudeDriver(runner)
	driver.tempRoot = t.TempDir()
	driver.removeAll = func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		return errors.New("cleanup-detail-should-not-escape")
	}
	credential, err := driver.Login(
		context.Background(), testClaudeExecutable(t), claudeTestStreams(),
	)
	if !errors.Is(err, ErrClaudeLoginCleanup) || len(credential.Token()) != 0 || credential.AccountLabel() != "" {
		t.Fatalf("credential=%v error=%v", credential, err)
	}
	if strings.Contains(err.Error(), "cleanup-detail-should-not-escape") || strings.Contains(err.Error(), claudeTokenCanary) {
		t.Fatalf("cleanup error leaked detail: %q", err)
	}
}

func TestClaudeLoginRequiresStreamsBeforeExecution(t *testing.T) {
	runner := &fakeClaudeRunner{}
	driver := NewClaudeDriver(runner)
	_, err := driver.Login(context.Background(), testClaudeExecutable(t), ClaudeLoginStreams{})
	if !errors.Is(err, ErrClaudeTTYRequired) || runner.callCount() != 0 {
		t.Fatalf("error=%v calls=%d", err, runner.callCount())
	}
}

func TestClaudeLoginRejectsUninitializedDriver(t *testing.T) {
	var nilDriver *ClaudeDriver
	if _, err := nilDriver.Login(
		context.Background(), testClaudeExecutable(t), claudeTestStreams(),
	); !errors.Is(err, ErrClaudeLoginSetup) {
		t.Fatalf("nil driver error = %v", err)
	}
	zeroDriver := &ClaudeDriver{}
	if _, err := zeroDriver.Login(
		context.Background(), testClaudeExecutable(t), claudeTestStreams(),
	); !errors.Is(err, ErrClaudeLoginSetup) {
		t.Fatalf("zero driver error = %v", err)
	}
}

func TestClaudeLoginRequiresAbsoluteExecutableAndBoundsItsOwnTimeout(t *testing.T) {
	t.Run("relative executable", func(t *testing.T) {
		runner := &fakeClaudeRunner{}
		driver := NewClaudeDriver(runner)
		_, err := driver.Login(context.Background(), "claude", claudeTestStreams())
		if !errors.Is(err, ErrClaudeExecutable) || runner.callCount() != 0 {
			t.Fatalf("error=%v calls=%d", err, runner.callCount())
		}
	})

	t.Run("internal timeout", func(t *testing.T) {
		runner := &fakeClaudeRunner{run: func(call int, ctx context.Context, command ClaudeCommand) error {
			if call == 0 {
				_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}}
		driver := NewClaudeDriver(runner)
		driver.tempRoot = t.TempDir()
		driver.timeout = 20 * time.Millisecond
		started := time.Now()
		_, err := driver.Login(context.Background(), testClaudeExecutable(t), claudeTestStreams())
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
			t.Fatalf("error=%v elapsed=%s", err, time.Since(started))
		}
	})
}

func TestClaudeDriverUsesInjectedParserSeamAndStillCopiesCredential(t *testing.T) {
	parser := &sentinelClaudeParser{token: []byte(claudeTokenCanary)}
	runner := &fakeClaudeRunner{run: func(call int, _ context.Context, command ClaudeCommand) error {
		if call == 0 {
			_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
			return err
		}
		_, err := command.Stdout.Write([]byte("synthetic terminal bytes"))
		return err
	}}
	driver := NewClaudeDriver(runner)
	driver.tempRoot = t.TempDir()
	driver.parserFactory = func(io.Writer) claudeOutputParser { return parser }
	credential, err := driver.Login(
		context.Background(), testClaudeExecutable(t), claudeTestStreams(),
	)
	if err != nil || !parser.finished || string(credential.Token()) != claudeTokenCanary {
		t.Fatalf("credential=%v parser finished=%t error=%v", credential, parser.finished, err)
	}
	credential.Clear()
}

func TestClaudeSetupOutputParserAcceptsOnlyPinnedSuccessBlockAndSuppressesToken(t *testing.T) {
	var visible bytes.Buffer
	parser := newClaudeSetupOutputParser(&visible)
	for _, chunk := range successfulClaudeOutputChunks(claudeTokenCanary) {
		for len(chunk) > 0 {
			width := 3
			if len(chunk) < width {
				width = len(chunk)
			}
			if _, err := parser.Write(chunk[:width]); err != nil {
				t.Fatal(err)
			}
			chunk = chunk[width:]
		}
	}
	if visible.Len() != 0 {
		t.Fatalf("provider output crossed the visible boundary before validation: %q", visible.String())
	}
	token, err := parser.Finish()
	if err != nil || string(token) != claudeTokenCanary {
		t.Fatalf("token=%q error=%v", token, err)
	}
	defer clear(token)
	if strings.Contains(visible.String(), claudeTokenCanary) || strings.Contains(visible.String(), claudeSetupTokenMarker) {
		t.Fatalf("secret phase reached visible output: %q", visible.String())
	}
	if _, err := parser.Write([]byte("after finish")); !errors.Is(err, ErrClaudeOutputFraming) {
		t.Fatalf("write-after-finish error = %v", err)
	}
	if token, err := parser.Finish(); !errors.Is(err, ErrClaudeOutputFraming) || token != nil {
		t.Fatalf("second finish token=%q error=%v", token, err)
	}
}

func TestClaudeSetupOutputParserAppliesPinnedInkEraseAndCursorRedraw(t *testing.T) {
	var visible bytes.Buffer
	parser := newClaudeSetupOutputParser(&visible)
	redrawn := []byte(
		"Waiting for browser login..." +
			"\x1b[2K\x1b[1G\x1b[32m" + claudeSetupSuccessLine + "\x1b[0m\r\n" +
			claudeSetupTokenMarker + "\r\n" +
			claudeTokenCanary + "\r\n" +
			claudeSetupFooter + "\r\n" +
			claudeSetupUsage + "\r\n",
	)
	if _, err := parser.Write(redrawn); err != nil {
		t.Fatal(err)
	}
	token, err := parser.Finish()
	if err != nil || string(token) != claudeTokenCanary {
		t.Fatalf("token=%q error=%v", token, err)
	}
	clear(token)
	if strings.Contains(visible.String(), claudeTokenCanary) {
		t.Fatalf("redrawn candidate reached visible output: %q", visible.String())
	}
}

func TestClaudeSetupOutputParserFailsClosedOnUnknownTerminalFraming(t *testing.T) {
	tests := map[string][]byte{
		"unknown OSC":       []byte("visible\x1b]8;;https://example.com\a"),
		"unknown CSI":       []byte("visible\x1b[?999h"),
		"alternate screen":  []byte("visible\x1b[?1049h"),
		"device query":      []byte("visible\x1b[6n"),
		"legacy escape":     []byte("visible\x1b7"),
		"bare control":      []byte("visible\a"),
		"tab":               []byte("visible\t"),
		"invalid UTF-8":     {0xff},
		"unfinished escape": []byte("visible\x1b["),
		"bidi format":       []byte("visible\u202e"),
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			parser := newClaudeSetupOutputParser(io.Discard)
			_, writeErr := parser.Write(output)
			token, finishErr := parser.Finish()
			if token != nil || (!errors.Is(writeErr, ErrClaudeOutputFraming) && !errors.Is(finishErr, ErrClaudeOutputFraming)) {
				t.Fatalf("token=%q write=%v finish=%v", token, writeErr, finishErr)
			}
		})
	}
}

func TestParseClaudeSetupTokenRejectsAmbiguousMalformedOrUnboundedCandidates(t *testing.T) {
	valid := plainClaudeSuccessOutput(claudeTokenCanary)
	if token, err := parseClaudeSetupToken(valid); err != nil || string(token) != claudeTokenCanary {
		t.Fatalf("valid token=%q error=%v", token, err)
	}
	tests := map[string][]byte{
		"empty":                  nil,
		"missing success":        bytes.Replace(valid, []byte(claudeSetupSuccessLine+"\n"), nil, 1),
		"success after marker":   []byte(claudeSetupTokenMarker + "\n" + claudeSetupSuccessLine + "\n" + claudeTokenCanary + "\n" + claudeSetupFooter + "\n" + claudeSetupUsage + "\n"),
		"missing candidate":      bytes.Replace(valid, []byte(claudeTokenCanary+"\n"), nil, 1),
		"second candidate":       bytes.Replace(valid, []byte(claudeSetupFooter), []byte("second-candidate\n"+claudeSetupFooter), 1),
		"duplicate marker":       append(append([]byte(nil), valid...), valid...),
		"short candidate":        bytes.Replace(valid, []byte(claudeTokenCanary), []byte("1234567"), 1),
		"candidate with space":   bytes.Replace(valid, []byte(claudeTokenCanary), []byte("token value"), 1),
		"candidate with control": bytes.Replace(valid, []byte(claudeTokenCanary), []byte("token\x7fvalue"), 1),
		"oversize candidate":     plainClaudeSuccessOutput(strings.Repeat("x", maxClaudeTokenBytes+1)),
		"wrong footer":           bytes.Replace(valid, []byte(claudeSetupFooter), []byte("Store it somewhere."), 1),
		"wrong usage":            bytes.Replace(valid, []byte(claudeSetupUsage), []byte("export TOKEN=<token>"), 1),
		"unknown prefix":         append([]byte("provider text\n"), valid...),
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			if token, err := parseClaudeSetupToken(output); !errors.Is(err, ErrClaudeTokenCapture) || token != nil {
				t.Fatalf("token=%q error=%v", token, err)
			}
		})
	}
}

// The pinned 2.1.220 token slot accepts one opaque 8..16384-byte value composed
// only of printable non-space ASCII. The surrounding terminal frame is exact;
// token identity does not depend on an undocumented value prefix.
func TestParseClaudeSetupTokenAcceptsExactDocumentedOpaqueGrammarBounds(t *testing.T) {
	printable := make([]byte, 0, 94)
	for current := byte(0x21); current <= 0x7e; current++ {
		printable = append(printable, current)
	}
	for name, token := range map[string]string{
		"minimum":         "12345678",
		"common prefix":   claudeTokenCanary,
		"printable ASCII": string(printable),
		"maximum":         strings.Repeat("x", maxClaudeTokenBytes),
	} {
		t.Run(name, func(t *testing.T) {
			captured, err := parseClaudeSetupToken(plainClaudeSuccessOutput(token))
			if err != nil || string(captured) != token {
				t.Fatalf("captured length=%d error=%v", len(captured), err)
			}
			clear(captured)
		})
	}
}

func TestClaudeSetupOutputParserKeepsMaximumCandidateOnOneFixedPTYLine(t *testing.T) {
	want := strings.Repeat("x", maxClaudeTokenBytes)
	parser := newClaudeSetupOutputParser(io.Discard)
	if _, err := parser.Write(plainClaudeSuccessOutput(want)); err != nil {
		t.Fatal(err)
	}
	token, err := parser.Finish()
	if err != nil || len(token) != maxClaudeTokenBytes || string(token) != want {
		t.Fatalf("captured length=%d error=%v", len(token), err)
	}
	clear(token)
}

func TestClaudeSetupOutputParserBoundsOutputAndVisibleDelivery(t *testing.T) {
	t.Run("terminal", func(t *testing.T) {
		parser := newClaudeSetupOutputParser(io.Discard)
		if _, err := parser.Write(bytes.Repeat([]byte("x"), maxClaudeTerminalBytes+1)); !errors.Is(err, ErrClaudeOutputLimit) {
			t.Fatalf("write error = %v", err)
		}
		if _, err := parser.Finish(); !errors.Is(err, ErrClaudeOutputLimit) {
			t.Fatalf("finish error = %v", err)
		}
	})
	t.Run("terminal cell amplification", func(t *testing.T) {
		for name, input := range map[string][]byte{
			"rows":             bytes.Repeat([]byte("\x1b[32767Gx\n"), 5),
			"erase reuse":      bytes.Repeat([]byte("\x1b[32767Gx\x1b[2J"), 5),
			"wide line erases": append([]byte("\x1b[32767Gx"), bytes.Repeat([]byte("\x1b[1K"), 4)...),
			"row vector reuse": bytes.Repeat([]byte("\x1b[2048H\x1b[2J"), 65),
		} {
			t.Run(name, func(t *testing.T) {
				parser := newClaudeSetupOutputParser(io.Discard)
				if len(input) >= maxClaudeTerminalBytes {
					t.Fatalf("adversarial input unexpectedly large: %d bytes", len(input))
				}
				if _, err := parser.Write(input); !errors.Is(err, ErrClaudeOutputLimit) {
					t.Fatalf("write error = %v", err)
				}
				if _, err := parser.Finish(); !errors.Is(err, ErrClaudeOutputLimit) {
					t.Fatalf("finish error = %v", err)
				}
			})
		}
	})
	t.Run("destination", func(t *testing.T) {
		parser := newClaudeSetupOutputParser(errorWriter{})
		if _, err := parser.Write(plainClaudeSuccessOutput(claudeTokenCanary)); err != nil {
			t.Fatalf("write error = %v", err)
		}
		if _, err := parser.Finish(); !errors.Is(err, ErrClaudeVisibleOutput) {
			t.Fatalf("finish error = %v", err)
		}
	})
}

func TestClaudeSetupOutputParserRejectsPreMarkerTokenEchoWithoutVisibleBytes(t *testing.T) {
	valid := plainClaudeSuccessOutput(claudeTokenCanary)
	tests := map[string][]byte{
		"plain echo": append(
			[]byte(claudeTokenCanary+"\n"), valid...,
		),
		"erased redraw echo": append(
			[]byte(claudeTokenCanary+"\x1b[2K\x1b[1G"), valid...,
		),
		"unknown pre-marker line": append(
			[]byte("unrecognized provider text\n"), valid...,
		),
		"cursor-positioned erased echo": append(
			append(reversePositionedClaudeRows(claudeTokenCanary), []byte("\x1b[2J")...), valid...,
		),
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			var visible bytes.Buffer
			parser := newClaudeSetupOutputParser(&visible)
			for _, current := range output {
				if _, err := parser.Write([]byte{current}); err != nil {
					t.Fatal(err)
				}
			}
			if visible.Len() != 0 {
				t.Fatalf("unvalidated transcript became visible: %q", visible.String())
			}
			if token, err := parser.Finish(); !errors.Is(err, ErrClaudeTokenCapture) || token != nil {
				t.Fatalf("token=%q error=%v", token, err)
			}
			if strings.Contains(visible.String(), claudeTokenCanary) || visible.Len() != 0 {
				t.Fatalf("rejected transcript leaked visible bytes: %q", visible.String())
			}
		})
	}
}

func TestClaudeSetupOutputParserRejectsErasedDistinctSuccessCandidate(t *testing.T) {
	firstToken := "sk-ant-oat01-erased_candidate_1234567890"
	positioned := []byte(
		claudeSetupSuccessLine + "\x1b[2H" +
			claudeSetupTokenMarker + "\x1b[3H" +
			firstToken + "\x1b[4H" +
			claudeSetupFooter + "\x1b[5H" +
			claudeSetupUsage,
	)
	reversePositioned := reversePositionedClaudeRows(
		claudeSetupSuccessLine,
		claudeSetupTokenMarker,
		firstToken,
		claudeSetupFooter,
		claudeSetupUsage,
	)
	for name, first := range map[string][]byte{
		"line separated":            plainClaudeSuccessOutput(firstToken),
		"cursor positioned":         positioned,
		"reverse cursor positioned": reversePositioned,
	} {
		t.Run(name, func(t *testing.T) {
			output := append(append([]byte(nil), first...), []byte("\x1b[2J")...)
			output = append(output, plainClaudeSuccessOutput(claudeTokenCanary)...)
			parser := newClaudeSetupOutputParser(io.Discard)
			if _, err := parser.Write(output); err != nil {
				t.Fatal(err)
			}
			if token, err := parser.Finish(); !errors.Is(err, ErrClaudeTokenCapture) || token != nil {
				t.Fatalf("token=%q error=%v", token, err)
			}
		})
	}
}

func TestClaudeSetupOutputParserAcceptsTokenThatIsFixedTextSubstring(t *testing.T) {
	for _, token := range []string{"authentication", "successfully!"} {
		t.Run(token, func(t *testing.T) {
			var visible bytes.Buffer
			parser := newClaudeSetupOutputParser(&visible)
			if _, err := parser.Write(plainClaudeSuccessOutput(token)); err != nil {
				t.Fatal(err)
			}
			captured, err := parser.Finish()
			if err != nil || string(captured) != token {
				t.Fatalf("token=%q error=%v", captured, err)
			}
			if bytes.Contains(visible.Bytes(), captured) {
				t.Fatalf("fixed guidance exposed token %q: %q", captured, visible.String())
			}
			clear(captured)
		})
	}
}

func TestClaudeSetupOutputParserNeverForwardsCandidateOnMalformedFooter(t *testing.T) {
	var visible bytes.Buffer
	parser := newClaudeSetupOutputParser(&visible)
	output := bytes.Replace(
		plainClaudeSuccessOutput(claudeTokenCanary),
		[]byte(claudeSetupFooter), []byte("malformed footer"), 1,
	)
	if _, err := parser.Write(output); err != nil {
		t.Fatal(err)
	}
	if token, err := parser.Finish(); !errors.Is(err, ErrClaudeTokenCapture) || token != nil {
		t.Fatalf("token=%q error=%v", token, err)
	}
	if strings.Contains(visible.String(), claudeTokenCanary) || strings.Contains(visible.String(), "malformed footer") {
		t.Fatalf("candidate or post-marker data escaped: %q", visible.String())
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic visible failure") }

type sentinelClaudeParser struct {
	token    []byte
	finished bool
}

func (p *sentinelClaudeParser) Write(content []byte) (int, error) { return len(content), nil }

func (p *sentinelClaudeParser) Finish() ([]byte, error) {
	p.finished = true
	return append([]byte(nil), p.token...), nil
}

func successfulClaudeRunner(t *testing.T, token string) *fakeClaudeRunner {
	t.Helper()
	return &fakeClaudeRunner{run: func(call int, _ context.Context, command ClaudeCommand) error {
		switch call {
		case 0:
			_, err := command.Stdout.Write([]byte(claudeVersionOutput + "\n"))
			return err
		case 1:
			for _, chunk := range successfulClaudeOutputChunks(token) {
				if _, err := command.Stdout.Write(chunk); err != nil {
					return err
				}
			}
			return nil
		default:
			t.Fatalf("unexpected command call %d", call)
			return nil
		}
	}}
}

func successfulClaudeOutputChunks(token string) [][]byte {
	return [][]byte{
		[]byte("\x1b[?25lThis will guide you through long-lived (1-year) auth token setup for your Claude account. Claude subscription required.\r\n"),
		[]byte("\x1b[32m" + claudeSetupSuccessLine + "\x1b[0m\r\n" + claudeSetupTokenMarker[:17]),
		[]byte(claudeSetupTokenMarker[17:] + "\r\n" + token[:11]),
		[]byte(token[11:] + "\r\n" + claudeSetupFooter + "\r\n"),
		[]byte(claudeSetupUsage + "\r\n\x1b[?25h"),
	}
}

func plainClaudeSuccessOutput(token string) []byte {
	return []byte(
		claudeSetupSuccessLine + "\n" +
			claudeSetupTokenMarker + "\n" +
			token + "\n" +
			claudeSetupFooter + "\n" +
			claudeSetupUsage + "\n",
	)
}

func reversePositionedClaudeRows(lines ...string) []byte {
	var output bytes.Buffer
	for row, line := range lines {
		runes := []rune(line)
		for column := len(runes) - 1; column >= 0; column-- {
			_, _ = fmt.Fprintf(&output, "\x1b[%d;%dH%s", row+1, column+1, string(runes[column]))
		}
	}
	return output.Bytes()
}

func claudeTestStreams() ClaudeLoginStreams {
	return ClaudeLoginStreams{Stdin: strings.NewReader("trusted terminal input"), Output: io.Discard}
}

func testClaudeExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("synthetic claude executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
