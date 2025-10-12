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

	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/supervisor"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func TestRunSession_ErrContextDone(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
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
		Kind:        api.RunNewSession,
		ID:          api.ID(naming.RandomID()),
		Name:        naming.RandomName(),
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
		SessionSpec: nil,
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
		t.Fatal("timeout waiting for runSession to return after close")
	}
}

func TestRunSession_ErrWaitOnReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
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
		Kind:        api.RunNewSession,
		ID:          api.ID(naming.RandomID()),
		Name:        naming.RandomName(),
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
		SessionSpec: nil,
	}
	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec) // will block until ctx.Done()
	}()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, errdefs.ErrWaitOnReady) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSession to return after SIGTERM")
	}
}

func TestRunSession_ErrWaitOnClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
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
		Kind:        api.RunNewSession,
		ID:          api.ID(naming.RandomID()),
		Name:        naming.RandomName(),
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
		SessionSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec) // will block until ctx.Done()
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-done; err != nil && !errors.Is(err, errdefs.ErrWaitOnClose) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
	}
}

func TestRunSession_ErrChildExit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newSupervisorController := func(_ context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
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
		Kind:        api.RunNewSession,
		ID:          api.ID(naming.RandomID()),
		Name:        naming.RandomName(),
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
		SessionSpec: nil,
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSupervisor(ctx, cancel, logger, ctrl, spec) // will block until ctx.Done()
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-done; err != nil && !errors.Is(err, errdefs.ErrChildExit) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrChildExit, err)
	}
}
