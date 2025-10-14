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

package session

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/session/sessionrpc"
	"github.com/eminwux/sbsh/internal/session/sessionrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

// (fakeListener and newStubListener removed as unused)

func Test_ErrSpecCmdMissing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(context.Background(), logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, _ *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSpecCmdMissing) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSpecCmdMissing, err)
	}
}

func Test_ErrOpenSocketCtrl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, _ *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
				return errors.New("error opening listener")
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}
}

func Test_ErrStartRPCServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- errors.New("make server fail")
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)
	defer close(exitCh)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartRPCServer, err)
	}
}

func Test_ErrStartSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(_ chan<- sessionrunner.SessionRunnerEvent) error {
				return errors.New("make start session fail")
			},
		}
	}
	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartSession) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartSession, err)
	}
}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(_ chan<- sessionrunner.SessionRunnerEvent) error {
				return nil
			},
			SetupShellFunc: func() error {
				return nil
			},
			OnInitShellFunc: func() error {
				return nil
			},
		}
	}
	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	time.Sleep(500 * time.Microsecond)

	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
	}
}

func Test_ErrRPCServerExited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(_ chan<- sessionrunner.SessionRunnerEvent) error {
				return nil
			},
			SetupShellFunc: func() error {
				return nil
			},
			OnInitShellFunc: func() error {
				return nil
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	sessionCtrl.rpcDoneCh <- errors.New("make rpc server exit with error")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}
}

func Test_WaitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(_ chan<- sessionrunner.SessionRunnerEvent) error {
				return nil
			},
			SetupShellFunc: func() error {
				return nil
			},
			OnInitShellFunc: func() error {
				return nil
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	readyReturn := make(chan error)

	go func() {
		readyReturn <- sessionCtrl.WaitReady()
	}()

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	if err := <-readyReturn; err != nil {
		t.Fatalf("expected 'nil'; got: '%v'", err)
	}
	cancel()
	<-exitCh
}

func Test_HandleEvent_EvCmdExited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(_ chan<- sessionrunner.SessionRunnerEvent) error {
				return nil
			},
			SetupShellFunc: func() error {
				return nil
			},
			OnInitShellFunc: func() error {
				return nil
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	readyReturn := make(chan error)
	defer close(readyReturn)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	ev := sessionrunner.SessionRunnerEvent{
		ID:   spec.ID,
		Type: sessionrunner.EvCmdExited,
		Err:  errors.New("session has been closed"),
		When: time.Now(),
	}

	sessionCtrl.eventsCh <- ev

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_HandleEvent_EvError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sessionCtrl := NewSessionController(ctx, logger).(*SessionController)
	sessionCtrl.NewSessionRunner = func(_ context.Context, _ *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *sessionrpc.SessionControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(_ chan<- sessionrunner.SessionRunnerEvent) error {
				return nil
			},
			SetupShellFunc: func() error {
				return nil
			},
			OnInitShellFunc: func() error {
				return nil
			},
		}
	}

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	readyReturn := make(chan error)
	defer close(readyReturn)

	go func() {
		exitCh <- sessionCtrl.Run(&spec)
	}()

	ev := sessionrunner.SessionRunnerEvent{
		ID:   spec.ID,
		Type: sessionrunner.EvError,
		Err:  errors.New("session has been closed"),
		When: time.Now(),
	}

	sessionCtrl.eventsCh <- ev
}
