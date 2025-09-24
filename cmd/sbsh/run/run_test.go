package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/session"
	"testing"
	"time"
)

func TestRunSession_ErrContextCancelled(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, cancel context.CancelFunc) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func(spec *api.SessionSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return nil },
			StatusFunc:    func() string { return "" },
		}
	}
	t.Cleanup(func() { newSessionController = orig })

	done := make(chan error)
	go func() {
		done <- runSession("s-ctx", "/bin/true", nil) // will block until ctx.Done()
	}()

	// Give Run() time to set ready, then signal the process (NotifyContext listens to SIGTERM/INT)
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, errdefs.ErrContextDone) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSession to return after SIGTERM")
	}

}

func TestRunSession_ErrWaitOnReady(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, cancel context.CancelFunc) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func(spec *api.SessionSpec) error { return nil },
			WaitReadyFunc: func() error { return fmt.Errorf("not ready") },
			WaitCloseFunc: func() error { return nil },
		}
	}
	t.Cleanup(func() { newSessionController = orig })

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	if err := runSession("s-wre", "/bin/true", nil); err != nil && !errors.Is(err, errdefs.ErrWaitOnReady) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)

	}
}

func TestRunSession_ErrWaitOnClose(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, cancel context.CancelFunc) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func(spec *api.SessionSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return fmt.Errorf("error on close") },
		}
	}
	t.Cleanup(func() { newSessionController = orig })

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- runSession("s-wre", "/bin/true", nil)
	}(exitCh)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrWaitOnClose) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
	}
}
