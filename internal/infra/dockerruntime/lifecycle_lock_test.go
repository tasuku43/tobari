package dockerruntime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestWithLifecycleLockSerializesOperations(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- runtime.WithLifecycleLock(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- runtime.WithLifecycleLock(context.Background(), func(context.Context) error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second lifecycle operation entered before the first released the lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second lifecycle operation did not enter after release")
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestWithLifecycleLockHonorsCancellationWhileWaiting(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- runtime.WithLifecycleLock(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	secondEntered := false
	err = runtime.WithLifecycleLock(ctx, func(context.Context) error {
		secondEntered = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || secondEntered {
		t.Fatalf("canceled wait err=%v entered=%v", err, secondEntered)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}
