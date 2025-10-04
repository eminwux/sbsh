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

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"sbsh/pkg/api"
	"sbsh/pkg/supervisor"
)

func TestRunSession_ErrContextDone(t *testing.T) {
	orig := newSupervisorController
	newSupervisorController = func(ctx context.Context) api.SupervisorController {
		return &supervisor.SupervisorControllerTest{
			RunFunc: func(spec *api.SupervisorSpec) error {
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
			RunFunc: func(spec *api.SupervisorSpec) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func() error {
				return fmt.Errorf("not ready")
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
			RunFunc: func(spec *api.SupervisorSpec) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return fmt.Errorf("not closed")
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
			RunFunc: func(spec *api.SupervisorSpec) error {
				// default: succeed without doing anything
				return fmt.Errorf("force child exit")
			},
			WaitReadyFunc: func() error {
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
