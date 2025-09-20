/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor"
	"testing"
	"time"
)

func TestRunSession_ErrContextDone(t *testing.T) {
	orig := newSupervisorController
	newSupervisorController = func(ctx context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
			RunFunc: func(ctx context.Context) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func(ctx context.Context) error {
				// default: succeed immediately
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			SetCurrentSessionFn: func(id api.SessionID) error {
				// default: just accept the ID
				return nil
			},
			StartFunc: func() error {
				// default: succeed immediately
				return nil
			},
		}
	}
	t.Cleanup(func() { newSupervisorController = orig })

	done := make(chan error)
	go func() {
		done <- runSupervisor() // will block until ctx.Done()
	}()

	// Give Run() time to set ready, then signal the process (NotifyContext listens to SIGTERM/INT)
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, ErrContextDone) {
			t.Fatalf("expected '%v'; got: '%v'", ErrContextDone, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSession to return after close")
	}

}

func TestRunSession_ErrWaitOnReady(t *testing.T) {
	orig := newSupervisorController
	newSupervisorController = func(ctx context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
			RunFunc: func(ctx context.Context) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func(ctx context.Context) error {
				return fmt.Errorf("not ready")
			},
			WaitCloseFunc: func() error {
				return nil
			},
			SetCurrentSessionFn: func(id api.SessionID) error {
				// default: just accept the ID
				return nil
			},
			StartFunc: func() error {
				// default: succeed immediately
				return nil
			},
		}
	}
	t.Cleanup(func() { newSupervisorController = orig })

	done := make(chan error)
	go func() {
		done <- runSupervisor() // will block until ctx.Done()
	}()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, ErrWaitOnReady) {
			t.Fatalf("expected '%v'; got: '%v'", ErrWaitOnReady, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSession to return after SIGTERM")
	}
}

func TestRunSession_ErrWaitOnClose(t *testing.T) {
	orig := newSupervisorController
	newSupervisorController = func(ctx context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
			RunFunc: func(ctx context.Context) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func(ctx context.Context) error {
				return nil
			},
			WaitCloseFunc: func() error {
				return fmt.Errorf("not closed")
			},
			SetCurrentSessionFn: func(id api.SessionID) error {
				// default: just accept the ID
				return nil
			},
			StartFunc: func() error {
				// default: succeed immediately
				return nil
			},
		}
	}
	t.Cleanup(func() { newSupervisorController = orig })

	done := make(chan error)
	go func() {
		done <- runSupervisor() // will block until ctx.Done()
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-done; err != nil && !errors.Is(err, ErrWaitOnClose) {
		t.Fatalf("expected '%v'; got: '%v'", ErrWaitOnClose, err)
	}
}

func TestRunSession_ErrChildExit(t *testing.T) {
	orig := newSupervisorController
	newSupervisorController = func(ctx context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
			RunFunc: func(ctx context.Context) error {
				// default: succeed without doing anything
				return fmt.Errorf("force child exit")
			},
			WaitReadyFunc: func(ctx context.Context) error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			SetCurrentSessionFn: func(id api.SessionID) error {
				// default: just accept the ID
				return nil
			},
			StartFunc: func() error {
				// default: succeed immediately
				return nil
			},
		}
	}
	t.Cleanup(func() { newSupervisorController = orig })

	done := make(chan error)
	go func() {
		done <- runSupervisor() // will block until ctx.Done()
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-done; err != nil && !errors.Is(err, ErrChildExit) {
		t.Fatalf("expected '%v'; got: '%v'", ErrChildExit, err)
	}
}
