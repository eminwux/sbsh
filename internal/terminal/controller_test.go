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

package terminal

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

// (fakeListener and newStubListener removed as unused)

func Test_ErrSpecCmdMissing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(context.Background(), logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, _ *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
		}
	}

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSpecCmdMissing) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSpecCmdMissing, err)
	}
}

func Test_ErrWriteMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			CreateMetadataFunc: func() error {
				return errors.New("error creating metadata file")
			},
		}
	}

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrWriteMetadata) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWriteMetadata, err)
	}
}

func Test_ErrOpenSocketCtrl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, _ *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
				return errors.New("error opening listener")
			},
		}
	}

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}
}

func Test_ErrStartRPCServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- errors.New("make server fail")
			},
		}
	}

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)
	defer close(exitCh)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartRPCServer, err)
	}
}

func Test_ErrStartTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
				return errors.New("make start terminal fail")
			},
		}
	}
	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartTerminal) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartTerminal, err)
	}
}

func Test_ErrSetupShell(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
				return nil
			},
			SetupShellFunc: func() error {
				return errors.New("make setup shell fail")
			},
		}
	}
	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSetupShell) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSetupShell, err)
	}
}

func Test_ErrInitShell(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
				return nil
			},
			SetupShellFunc: func() error {
				return nil
			},
			OnInitShellFunc: func() error {
				return errors.New("make init shell fail")
			},
		}
	}
	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrInitShell) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInitShell, err)
	}
}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
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
	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
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
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
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

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	terminalCtrl.rpcDoneCh <- errors.New("make rpc server exit with error")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}
}

func Test_WaitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
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

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	readyReturn := make(chan error)

	go func() {
		readyReturn <- terminalCtrl.WaitReady()
	}()

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	if err := <-readyReturn; err != nil {
		t.Fatalf("expected 'nil'; got: '%v'", err)
	}
	cancel()
	<-exitCh
}

func Test_ErrCloseReq(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
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

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	// Wait for controller to be ready
	time.Sleep(500 * time.Microsecond)

	// Send direct close request
	terminalCtrl.closeReqCh <- errors.New("direct close request")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_HandleEvent_EvCmdExited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
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

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	readyReturn := make(chan error)
	defer close(readyReturn)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	ev := terminalrunner.Event{
		ID:   spec.ID,
		Type: terminalrunner.EvCmdExited,
		Err:  errors.New("terminal has been closed"),
		When: time.Now(),
	}

	terminalCtrl.eventsCh <- ev

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_HandleEvent_EvError(_ *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	terminalCtrl := NewTerminalController(ctx, logger).(*Controller)
	terminalCtrl.NewTerminalRunner = func(_ context.Context, _ *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner {
		return &terminalrunner.Test{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *terminalrpc.TerminalControllerRPC, readyCh chan error, _ chan error) {
				readyCh <- nil
			},
			StartTerminalFunc: func(_ chan<- terminalrunner.Event) error {
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

	// Define a new Terminal
	spec := api.TerminalSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.TerminalLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	exitCh := make(chan error)

	readyReturn := make(chan error)
	defer close(readyReturn)

	go func() {
		exitCh <- terminalCtrl.Run(&spec)
	}()

	ev := terminalrunner.Event{
		ID:   spec.ID,
		Type: terminalrunner.EvError,
		Err:  errors.New("terminal has been closed"),
		When: time.Now(),
	}

	terminalCtrl.eventsCh <- ev
}
