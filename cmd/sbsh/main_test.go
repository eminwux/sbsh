/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"errors"
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
		t.Fatal("timeout waiting for runSession to return after SIGTERM")
	}

}
