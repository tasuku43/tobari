//go:build linux || darwin

package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

func TestHostAWSProfilePromptPTYModesRemainContextBounded(t *testing.T) {
	for _, test := range []struct {
		name     string
		deadline bool
	}{
		{name: "canonical_cancel"},
		{name: "canonical_deadline", deadline: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			master, slave := openHostLoginPTY(t)
			defer master.Close()
			defer slave.Close()
			var ctx context.Context
			var stop context.CancelFunc
			if test.deadline {
				ctx, stop = context.WithTimeout(context.Background(), 300*time.Millisecond)
			} else {
				ctx, stop = context.WithCancel(context.Background())
			}
			defer stop()
			acquirer := &fakeHostCredentialAcquirer{}
			runner := &hostLoginDockerRunner{}
			prompt := &promptSignalWriter{written: make(chan struct{})}
			runtime := &Runtime{
				runner:            runner,
				hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/aws"},
				credentialHost:    acquirer,
				hostLoginProfiles: hostLoginPTYProfileReader(nil),
				browser:           &recordingBrowser{},
			}
			result := make(chan error, 1)
			go func() {
				_, loginErr := runtime.runHostCredentialLoginOnTTY(
					ctx, hostLoginContextID, "aws", slave, prompt,
				)
				result <- loginErr
			}()

			select {
			case <-prompt.written:
			case <-time.After(time.Second):
				t.Fatal("AWS PTY profile prompt did not begin")
			}
			if _, err := io.WriteString(master, "x"); err != nil {
				t.Fatal(err)
			}
			if !test.deadline {
				stop()
			}
			started := time.Now()
			select {
			case loginErr := <-result:
				want := context.Canceled
				if test.deadline {
					want = context.DeadlineExceeded
				}
				if !errors.Is(loginErr, want) {
					t.Fatalf("login error = %v; want %v", loginErr, want)
				}
				if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
					t.Fatalf("PTY prompt context stop took %s", elapsed)
				}
			case <-time.After(time.Second):
				t.Fatal("AWS PTY profile prompt remained blocked after context stop")
			}
			if acquirer.awsCalls != 0 || runner.calls != 0 {
				t.Fatalf("AWS acquisitions=%d Broker mutations=%d", acquirer.awsCalls, runner.calls)
			}
		})
	}
}

func TestHostAWSProfilePromptReadinessFlushRemainsCancelable(t *testing.T) {
	master, slave := openHostLoginPTY(t)
	defer master.Close()
	defer slave.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	acquirer := &fakeHostCredentialAcquirer{}
	runner := &hostLoginDockerRunner{}
	prompt := &promptSignalWriter{written: make(chan struct{})}
	firstReady := make(chan *os.File, 1)
	releaseFirst := make(chan struct{})
	secondWait := make(chan struct{})
	waitCalls := 0
	profileReader := hostLoginPTYProfileReader(func(ctx context.Context, input io.Reader) error {
		waitCalls++
		if waitCalls == 2 {
			close(secondWait)
		}
		if err := waitHostLoginInput(ctx, input); err != nil {
			return err
		}
		if waitCalls == 1 {
			firstReady <- input.(*os.File)
			<-releaseFirst
		}
		return nil
	})
	runtime := &Runtime{
		runner:            runner,
		hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/aws"},
		credentialHost:    acquirer,
		hostLoginProfiles: profileReader,
		browser:           &recordingBrowser{},
	}
	result := make(chan error, 1)
	go func() {
		_, loginErr := runtime.runHostCredentialLoginOnTTY(
			ctx, hostLoginContextID, "aws", slave, prompt,
		)
		result <- loginErr
	}()

	select {
	case <-prompt.written:
	case <-time.After(time.Second):
		t.Fatal("AWS PTY profile prompt did not begin")
	}
	if _, err := io.WriteString(master, "ready line\n"); err != nil {
		t.Fatal(err)
	}
	var privateInput *os.File
	select {
	case privateInput = <-firstReady:
	case <-time.After(time.Second):
		t.Fatal("private PTY descriptor did not become readable")
	}
	var drained [64]byte
	if count, err := syscall.Read(int(privateInput.Fd()), drained[:]); err != nil || count == 0 {
		t.Fatalf("drain ready PTY input: count=%d error=%v", count, err)
	}
	close(releaseFirst)
	select {
	case <-secondWait:
	case <-time.After(time.Second):
		t.Fatal("EAGAIN did not return profile input to readiness polling")
	}
	started := time.Now()
	cancel()
	select {
	case loginErr := <-result:
		if !errors.Is(loginErr, context.Canceled) {
			t.Fatalf("login error = %v", loginErr)
		}
		if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
			t.Fatalf("readiness-flush cancellation took %s", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("flushed PTY profile prompt remained blocked after cancellation")
	}
	if acquirer.awsCalls != 0 || runner.calls != 0 {
		t.Fatalf("AWS acquisitions=%d Broker mutations=%d", acquirer.awsCalls, runner.calls)
	}
}

func TestHostAWSProfileReaderUsesPrivatePTYForCompleteProfile(t *testing.T) {
	master, slave := openHostLoginPTY(t)
	defer master.Close()
	defer slave.Close()
	prompt := &promptSignalWriter{written: make(chan struct{})}
	type profileResult struct {
		profile credentialhost.ProfileConfig
		err     error
	}
	result := make(chan profileResult, 1)
	go func() {
		profile, err := (osHostLoginProfileReader{}).ReadAWSProfile(
			context.Background(), slave, prompt,
		)
		result <- profileResult{profile: profile, err: err}
	}()
	select {
	case <-prompt.written:
	case <-time.After(time.Second):
		t.Fatal("AWS PTY profile prompt did not begin")
	}
	if _, err := io.WriteString(
		master,
		"https://example.awsapps.com/start\n"+
			"us-east-1\n"+
			"123456789012\n"+
			"Developer\n",
	); err != nil {
		t.Fatal(err)
	}
	select {
	case read := <-result:
		want := credentialhost.ProfileConfig{
			StartURL:  "https://example.awsapps.com/start",
			SSORegion: "us-east-1",
			AccountID: "123456789012",
			RoleName:  "Developer",
		}
		if read.err != nil || read.profile != want {
			t.Fatalf("profile=%+v error=%v", read.profile, read.err)
		}
	case <-time.After(time.Second):
		t.Fatal("AWS private PTY profile read did not complete")
	}
}

func TestHostAWSProfilePrivateInputDoesNotChangeInheritedFlags(t *testing.T) {
	master, slave := openHostLoginPTY(t)
	defer master.Close()
	defer slave.Close()
	before := hostLoginFileStatusFlags(t, slave)
	if before&syscall.O_NONBLOCK != 0 {
		t.Fatal("PTY fixture unexpectedly opened inherited input nonblocking")
	}
	privateInput, err := openHostLoginInput(slave)
	if err != nil {
		t.Fatal(err)
	}
	defer privateInput.Close()
	if privateFlags := hostLoginFileStatusFlags(t, privateInput); privateFlags&syscall.O_NONBLOCK == 0 {
		t.Fatal("private profile descriptor is not nonblocking")
	}
	if after := hostLoginFileStatusFlags(t, slave); after != before {
		t.Fatalf("inherited terminal flags changed: before=%#x after=%#x", before, after)
	}
}

func TestHostAWSProfilePromptNoncanonicalPTYFailsClosedBeforeRead(t *testing.T) {
	master, slave := openHostLoginPTY(t)
	defer master.Close()
	defer slave.Close()
	// This is the audited blocking mode: with one byte available, VMIN=5 and
	// VTIME=20 can hold a read for two seconds. Production must reject the mode
	// before beginning a prompt read.
	setHostLoginPTYNoncanonical(t, slave, 5, 20)
	if _, err := io.WriteString(master, "x"); err != nil {
		t.Fatal(err)
	}

	acquirer := &fakeHostCredentialAcquirer{}
	runner := &hostLoginDockerRunner{}
	runtime := &Runtime{
		runner:            runner,
		hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/aws"},
		credentialHost:    acquirer,
		hostLoginProfiles: osHostLoginProfileReader{},
		browser:           &recordingBrowser{},
	}
	started := time.Now()
	_, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "aws", slave, io.Discard,
	)
	if !errors.Is(err, errHostLoginPrompt) {
		t.Fatalf("login error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("noncanonical PTY rejection took %s", elapsed)
	}
	if acquirer.awsCalls != 0 || runner.calls != 0 {
		t.Fatalf("AWS acquisitions=%d Broker mutations=%d", acquirer.awsCalls, runner.calls)
	}
}

func hostLoginPTYProfileReader(waitInput func(context.Context, io.Reader) error) osHostLoginProfileReader {
	return osHostLoginProfileReader{waitInput: waitInput}
}
