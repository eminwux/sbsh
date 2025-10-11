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

package run

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/session"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func TestRunSession_ErrContextCancelled(t *testing.T) {
	newSessionController := func(_ context.Context) api.SessionController {
		return &session.FakeSessionController{
			Exit:          nil,
			RunFunc:       func(_ *api.SessionSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return nil },
			StatusFunc:    func() string { return "" },
		}
	}

	ctrl := newSessionController(context.Background())

	t.Cleanup(func() {})

	spec := api.SessionSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.SessionLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}

	done := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runSession(ctx, cancel, ctrl, &spec) // will block until ctx.Done()
		defer close(done)
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
	newSessionController := func(_ context.Context) api.SessionController {
		return &session.FakeSessionController{
			Exit:          nil,
			RunFunc:       func(_ *api.SessionSpec) error { return nil },
			WaitReadyFunc: func() error { return errors.New("not ready") },
			WaitCloseFunc: func() error { return nil },
		}
	}

	ctrl := newSessionController(context.Background())

	t.Cleanup(func() {})

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	spec := api.SessionSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.SessionLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := runSession(ctx, cancel, ctrl, &spec); err != nil && !errors.Is(err, errdefs.ErrWaitOnReady) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)
	}
}

func TestRunSession_ErrWaitOnClose(t *testing.T) {
	newSessionController := func(_ context.Context) api.SessionController {
		return &session.FakeSessionController{
			Exit:          nil,
			RunFunc:       func(_ *api.SessionSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return errors.New("error on close") },
		}
	}
	t.Cleanup(func() {})

	ctrl := newSessionController(context.Background())

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	spec := api.SessionSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.SessionLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())

	go func(exitCh chan error) {
		exitCh <- runSession(ctx, cancel, ctrl, &spec)
		defer close(exitCh)
	}(exitCh)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrWaitOnClose) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
	}
}
