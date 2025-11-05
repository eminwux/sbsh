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

package supervisor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrpc"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrunner"
	"github.com/eminwux/sbsh/internal/supervisor/terminalstore"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func Test_ErrOpenSocketCtrl(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return errors.New("force socket fail")
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Name:    "default",
		LogFile: "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	readyReturn := make(chan error)
	go func() {
		readyReturn <- sc.WaitReady()
	}()

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	select {
	case err := <-readyReturn:
		if err != nil {
			t.Fatalf("expected 'nil'; got: '%v'", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("WaitReady timed out")
	}

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}
}

func Test_ErrStartRPCServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- errors.New("force server fail"):
				default:
				}
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Name:    "default",
		LogFile: "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	readyReturn := make(chan error)
	go func() {
		readyReturn <- sc.WaitReady()
	}()

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	select {
	case err := <-readyReturn:
		if err != nil {
			t.Fatalf("expected 'nil'; got: '%v'", err)
		}
	case <-time.After(100 * time.Millisecond): // pick a sensible deadline
		t.Fatalf("WaitReady timed out")
	}

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartRPCServer, err)
	}
}

func Test_ErrAttach(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return errors.New("force terminal start fail")
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return errors.New("force add fail")
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Name:    "default",
		LogFile: "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
		Kind:    api.AttachToTerminal,
		TerminalSpec: &api.TerminalSpec{
			ID:   "term-1",
			Name: "terminal-1",
		},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrAttach) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrAttach, err)
	}
}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(ctx, logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		TerminalSpec: &api.TerminalSpec{},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	//	<-ctrlReady
	time.Sleep(10 * time.Millisecond)
	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
	}
}

func Test_ErrRPCServerExited(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	exitCh := make(chan error)

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		TerminalSpec: &api.TerminalSpec{},
	}
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	<-sc.ctrlReadyCh
	sc.rpcDoneCh <- errors.New("force rpc server exit")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}
}

func Test_ErrCloseReq(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	exitCh := make(chan error)

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		TerminalSpec: &api.TerminalSpec{},
	}

	go func() {
		exitCh <- sc.Run(&spec)
	}()

	<-sc.ctrlReadyCh
	sc.closeReqCh <- errors.New("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_ErrStartCmd(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return errors.New("force cmd start fail")
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	exitCh := make(chan error)

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		TerminalSpec: &api.TerminalSpec{},
	}

	go func() {
		exitCh <- sc.Run(&spec)
	}()

	//	<-ctrlReady

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartCmd) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartCmd, err)
	}
}

func Test_ErrTerminalStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(_ error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(_ api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
		}
	}
	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return errors.New("force add fail")
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	exitCh := make(chan error)

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		TerminalSpec: &api.TerminalSpec{},
	}

	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrTerminalStore) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrTerminalStore, err)
	}
}

func Test_ErrWriteMetadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			CreateMetadataFunc: func() error {
				return errors.New("force metadata creation fail")
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:     func() api.ID { return "" },
			CloseFunc:  func(_ error) error { return nil },
			ResizeFunc: func(_ api.ResizeArgs) {},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Name:    "default",
		LogFile: "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrWriteMetadata) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWriteMetadata, err)
	}
}

func Test_ErrNoTerminalSpec(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: nil, // This should trigger ErrNoTerminalSpec
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrNoTerminalSpec) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoTerminalSpec, err)
	}
}

func Test_ErrTerminalExists(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return errdefs.ErrTerminalExists
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil {
		// Should be wrapped with ErrTerminalStore
		if !errors.Is(err, errdefs.ErrTerminalStore) {
			t.Fatalf("expected error to wrap '%v'; got: '%v'", errdefs.ErrTerminalStore, err)
		}
		// Should also contain ErrTerminalExists
		if !errors.Is(err, errdefs.ErrTerminalExists) {
			t.Fatalf("expected error to wrap '%v'; got: '%v'", errdefs.ErrTerminalExists, err)
		}
	} else {
		t.Fatal("expected error but got nil")
	}
}

func Test_ErrAttachNoTerminalSpec(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.AttachToTerminal,
		TerminalSpec: nil, // This should trigger ErrAttachNoTerminalSpec
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrAttachNoTerminalSpec) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrAttachNoTerminalSpec, err)
	}
}

func Test_ErrAttachNoIDOrName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.AttachToTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "", Name: ""}, // Both empty should trigger ErrAttachNoTerminalSpec
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrAttachNoTerminalSpec) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrAttachNoTerminalSpec, err)
	}
}

func Test_ErrSupervisorKind(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.SupervisorKind(999), // Invalid kind
		TerminalSpec: &api.TerminalSpec{},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSupervisorKind) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSupervisorKind, err)
	}
}

func Test_EventCmdExited(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	closeTriggered := make(chan bool, 1)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				return "test-terminal"
			},
			CloseFunc: func(_ error) error {
				select {
				case closeTriggered <- true:
				default:
				}
				return nil
			},
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	// Wait for controller to be ready
	<-sc.ctrlReadyCh

	// Send EvCmdExited event
	ev := supervisorrunner.Event{
		ID:   "test-terminal",
		Type: supervisorrunner.EvCmdExited,
		Err:  nil,
		When: time.Now(),
	}
	sc.eventsCh <- ev

	// Wait for close to be triggered
	select {
	case <-closeTriggered:
		// Success - onClosed was called
		// The Close() call in onClosed will cause Run() to exit via closeReqCh
	case <-time.After(1 * time.Second):
		t.Fatal("onClosed was not called after EvCmdExited event")
	}

	// Wait for Run to exit (it will exit when Close sends to closeReqCh)
	<-exitCh
}

func Test_EventError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	closeTriggered := make(chan bool, 1)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				return "test-terminal"
			},
			CloseFunc: func(_ error) error {
				select {
				case closeTriggered <- true:
				default:
				}
				return nil
			},
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	// Wait for controller to be ready
	<-sc.ctrlReadyCh

	// Send EvError event
	ev := supervisorrunner.Event{
		ID:   "test-terminal",
		Type: supervisorrunner.EvError,
		Err:  errors.New("test error"),
		When: time.Now(),
	}
	sc.eventsCh <- ev

	// Wait for close to be triggered
	select {
	case <-closeTriggered:
		// Success - onClosed was called
		// The Close() call in onClosed will cause Run() to exit via closeReqCh
	case <-time.After(1 * time.Second):
		t.Fatal("onClosed was not called after EvError event")
	}

	// Wait for Run to exit (it will exit when Close sends to closeReqCh)
	<-exitCh
}

func Test_EventDetach(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	detachCalled := make(chan bool, 1)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				return "test-terminal"
			},
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			DetachFunc: func() error {
				select {
				case detachCalled <- true:
				default:
				}
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	// Wait for controller to be ready
	<-sc.ctrlReadyCh

	// Send EvDetach event
	ev := supervisorrunner.Event{
		ID:   "test-terminal",
		Type: supervisorrunner.EvDetach,
		Err:  nil,
		When: time.Now(),
	}
	sc.eventsCh <- ev

	// Wait for detach to be called
	select {
	case <-detachCalled:
		// Success - Detach was called
	case <-time.After(1 * time.Second):
		t.Fatal("Detach was not called after EvDetach event")
	}

	// Close the controller to clean up (EvDetach doesn't trigger Close, so we need to do it)
	_ = sc.Close(errors.New("test cleanup"))
	<-exitCh
}

func Test_EventDetachFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	detachCalled := make(chan bool, 1)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				return "test-terminal"
			},
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			DetachFunc: func() error {
				select {
				case detachCalled <- true:
				default:
				}
				return errors.New("detach failed")
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	// Wait for controller to be ready
	<-sc.ctrlReadyCh

	// Send EvDetach event
	ev := supervisorrunner.Event{
		ID:   "test-terminal",
		Type: supervisorrunner.EvDetach,
		Err:  nil,
		When: time.Now(),
	}
	sc.eventsCh <- ev

	// Wait for detach to be called (should fail but still be called)
	select {
	case <-detachCalled:
		// Success - Detach was called (even though it failed)
	case <-time.After(1 * time.Second):
		t.Fatal("Detach was not called after EvDetach event")
	}

	// Close the controller to clean up (EvDetach doesn't trigger Close, so we need to do it)
	_ = sc.Close(errors.New("test cleanup"))
	<-exitCh
}

func Test_EventUnknown(_ *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				return "test-terminal"
			},
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	// Wait for controller to be ready
	<-sc.ctrlReadyCh

	// Send unknown event type
	ev := supervisorrunner.Event{
		ID:   "test-terminal",
		Type: supervisorrunner.EventType(999), // Unknown event type
		Err:  nil,
		When: time.Now(),
	}
	sc.eventsCh <- ev

	// Wait a bit to ensure the event is processed
	time.Sleep(100 * time.Millisecond)

	// Close the controller to clean up
	_ = sc.Close(errors.New("test cleanup"))
	<-exitCh
}

func Test_WaitReadyContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(ctx, logger).(*Controller)

	// Cancel context before WaitReady can complete
	cancel()

	err := sc.WaitReady()
	if err == nil {
		t.Fatal("expected error from WaitReady when context is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error; got: '%v'", err)
	}
}

func Test_WaitClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			CloseFunc: func(_ error) error {
				return nil
			},
		}
	}

	// Initialize sr to avoid nil pointer
	sc.sr = sc.NewSupervisorRunner(sc.ctx, sc.logger, nil, sc.eventsCh)

	// Close the controller
	_ = sc.Close(errors.New("test close"))

	// WaitClose should complete without error
	done := make(chan error)
	go func() {
		done <- sc.WaitClose()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error from WaitClose; got: '%v'", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("WaitClose timed out")
	}
}

func Test_DetachSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			DetachFunc: func() error {
				return nil
			},
		}
	}

	sc.sr = sc.NewSupervisorRunner(sc.ctx, sc.logger, nil, sc.eventsCh)

	err := sc.Detach()
	if err != nil {
		t.Fatalf("expected nil error from Detach; got: '%v'", err)
	}
}

func Test_ErrDetachTerminal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			DetachFunc: func() error {
				return errors.New("detach failed")
			},
		}
	}

	sc.sr = sc.NewSupervisorRunner(sc.ctx, sc.logger, nil, sc.eventsCh)

	err := sc.Detach()
	if err == nil {
		t.Fatal("expected error from Detach")
	}
	if !errors.Is(err, errdefs.ErrDetachTerminal) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrDetachTerminal, err)
	}
}

func Test_CloseIdempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			CloseFunc: func(_ error) error {
				return nil
			},
		}
	}

	// Initialize sr to avoid nil pointer
	sc.sr = sc.NewSupervisorRunner(sc.ctx, sc.logger, nil, sc.eventsCh)

	// Start a goroutine to drain closingCh (simulating what Run() does)
	go func() {
		errC := <-sc.closingCh
		sc.shuttingDown = true
		sc.logger.Warn("controller closing", "reason", errC)
	}()

	// First close
	err1 := sc.Close(errors.New("first close"))
	if err1 != nil {
		t.Fatalf("expected nil error from first Close; got: '%v'", err1)
	}

	// Wait a bit for shuttingDown to be set
	time.Sleep(10 * time.Millisecond)

	// Second close should be idempotent
	err2 := sc.Close(errors.New("second close"))
	if err2 != nil {
		t.Fatalf("expected nil error from second Close; got: '%v'", err2)
	}

	// Verify shuttingDown is set
	if !sc.shuttingDown {
		t.Fatal("expected shuttingDown to be true after Close")
	}
}

func Test_CloseErrorHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			CloseFunc: func(_ error) error {
				return errors.New("close error from runner")
			},
		}
	}

	sc.sr = sc.NewSupervisorRunner(sc.ctx, sc.logger, nil, sc.eventsCh)

	// Start a goroutine to drain closingCh (simulating what Run() does)
	go func() {
		errC := <-sc.closingCh
		sc.shuttingDown = true
		sc.logger.Warn("controller closing", "reason", errC)
	}()

	// Close should still succeed even if sr.Close returns an error
	err := sc.Close(errors.New("test close"))
	if err != nil {
		t.Fatalf("expected nil error from Close even when sr.Close fails; got: '%v'", err)
	}

	// Wait a bit for shuttingDown to be set by the goroutine
	time.Sleep(10 * time.Millisecond)

	// Verify shuttingDown is set
	if !sc.shuttingDown {
		t.Fatal("expected shuttingDown to be true after Close")
	}
}

func Test_RunNewTerminalSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sc := NewSupervisorController(context.Background(), logger).(*Controller)
	sc.NewSupervisorRunner = func(ctx context.Context, logger *slog.Logger, _ *api.SupervisorSpec, _ chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.Test{
			Ctx:    ctx,
			Logger: logger,
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(_ context.Context, _ *supervisorrpc.SupervisorControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			IDFunc: func() api.ID {
				return "test-terminal"
			},
			CloseFunc:          func(_ error) error { return nil },
			ResizeFunc:         func(_ api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
			StartTerminalCmdFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
		}
	}

	sc.NewTerminalStore = func() terminalstore.TerminalStore {
		return &terminalstore.Test{
			AddFunc: func(_ *api.SupervisedTerminal) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedTerminal, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(_ api.ID) {},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:           api.ID(supervisorID),
		Name:         "default",
		LogFile:      "/tmp/sbsh-logs/s0",
		RunPath:      viper.GetString("global.runPath"),
		Kind:         api.RunNewTerminal,
		TerminalSpec: &api.TerminalSpec{ID: "test-terminal"},
	}

	readyCh := make(chan error)
	go func() {
		readyCh <- sc.WaitReady()
	}()

	exitCh := make(chan error)
	go func() {
		exitCh <- sc.Run(&spec)
	}()

	// Wait for ready
	select {
	case err := <-readyCh:
		if err != nil {
			t.Fatalf("expected nil error from WaitReady; got: '%v'", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("WaitReady timed out")
	}

	// Close the controller to clean up
	_ = sc.Close(errors.New("test cleanup"))
	<-exitCh
}
