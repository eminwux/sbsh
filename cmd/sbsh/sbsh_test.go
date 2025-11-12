// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package sbsh

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/supervisor"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func TestRunTerminal_ErrContextDone(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func() error {
				// default: succeed immediately
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				// default: succeed immediately
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec) // will block until ctx.Done()
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
		t.Fatal("timeout waiting for runSupervisor to return after close")
	}
}

func TestRunTerminal_ErrWaitOnReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	expectedErr := errdefs.ErrStartRPCServer
	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				return nil
			},
			WaitReadyFunc: func() error {
				return expectedErr
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}
	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrWaitOnReady) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)
		}
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", expectedErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSupervisor to return")
	}
}

func TestRunTerminal_ErrContextDoneWithWaitCloseError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	waitCloseErr := errdefs.ErrOnClose
	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				// Block to allow context cancellation
				time.Sleep(100 * time.Millisecond)
				return nil
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return waitCloseErr
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec)
	}()

	// Give Run() time to set ready, then cancel context
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrContextDone) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
		}
		if !errors.Is(err, errdefs.ErrWaitOnClose) {
			t.Fatalf("expected wrapped '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
		}
		if !errors.Is(err, waitCloseErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", waitCloseErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSupervisor to return")
	}
}

func TestRunTerminal_ErrChildExit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	runErr := errdefs.ErrRPCServerExited
	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				return runErr
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec)
	}()

	// Wait for Run() to complete and error to be handled
	select {
	case err := <-done:
		// runSupervisor returns nil when errCh receives error (line 412)
		// The error is logged but not returned to avoid polluting terminal
		if err != nil {
			t.Fatalf("expected nil (error logged but not returned); got: '%v'", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSupervisor to return")
	}
}

func TestRunTerminal_ErrChildExitWithWaitCloseError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	runErr := errdefs.ErrRPCServerExited
	waitCloseErr := errdefs.ErrOnClose
	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				return runErr
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return waitCloseErr
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec)
	}()

	// Wait for Run() to complete and error to be handled
	select {
	case err := <-done:
		// runSupervisor returns nil when errCh receives error (line 412)
		// The error is logged but not returned to avoid polluting terminal
		if err != nil {
			t.Fatalf("expected nil (error logged but not returned); got: '%v'", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSupervisor to return")
	}
}

func TestRunTerminal_SuccessWithNilError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				return nil
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec)
	}()

	// Wait for Run() to complete successfully
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error; got: '%v'", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSupervisor to return")
	}
}

func TestRunTerminal_SuccessWithContextCanceled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so Run() receives context.Canceled

	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.ControllerTest{
			RunFunc: func(_ *api.SupervisorSpec) error {
				return ctx.Err() // Return context.Canceled
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newSupervisorController(context.Background())

	t.Cleanup(func() {})

	// Define a new Supervisor
	spec := &api.SupervisorSpec{
		Kind:         api.RunNewTerminal,
		ID:           api.ID(naming.RandomID()),
		Name:         naming.RandomName(),
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
		TerminalSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	runCtx, runCancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(runCtx, runCancel, logger, ctrl, spec)
	}()

	// Wait for Run() to complete with context.Canceled
	select {
	case err := <-done:
		// When errCh receives context.Canceled, runSupervisor returns nil (line 414)
		if err != nil {
			t.Fatalf("expected nil (context.Canceled is ignored); got: '%v'", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSupervisor to return")
	}
}
