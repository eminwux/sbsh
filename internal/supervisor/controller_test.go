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
	"github.com/eminwux/sbsh/internal/supervisor/sessionstore"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrpc"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrunner"
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
			AttachFunc: func(_ *api.SupervisedSession) error {
				return errors.New("force session start fail")
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
			StartSessionCmdFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return errors.New("force add fail")
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		Kind:    api.AttachToSession,
		SessionSpec: &api.SessionSpec{
			ID:   "sess-1",
			Name: "session-1",
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
			AttachFunc: func(_ *api.SupervisedSession) error {
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
			StartSessionCmdFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		ID:          api.ID(supervisorID),
		Name:        "default",
		LogFile:     "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
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
			AttachFunc: func(_ *api.SupervisedSession) error {
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
			StartSessionCmdFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		ID:          api.ID(supervisorID),
		Name:        "default",
		LogFile:     "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
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

func Test_ErrSessionExists(t *testing.T) {
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
			AttachFunc: func(_ *api.SupervisedSession) error {
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
			StartSessionCmdFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
		}
	}
	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		ID:          api.ID(supervisorID),
		Name:        "default",
		LogFile:     "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
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
			AttachFunc: func(_ *api.SupervisedSession) error {
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
			StartSessionCmdFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		ID:          api.ID(supervisorID),
		Name:        "default",
		LogFile:     "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
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

func Test_ErrStartSessionCmd(t *testing.T) {
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
			AttachFunc: func(_ *api.SupervisedSession) error {
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
			StartSessionCmdFunc: func(_ *api.SupervisedSession) error {
				return errors.New("force cmd start fail")
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		ID:          api.ID(supervisorID),
		Name:        "default",
		LogFile:     "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
	}

	go func() {
		exitCh <- sc.Run(&spec)
	}()

	//	<-ctrlReady

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartSessionCmd) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartSessionCmd, err)
	}
}

func Test_ErrSessionStore(t *testing.T) {
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
			AttachFunc: func(_ *api.SupervisedSession) error {
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
	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.Test{
			AddFunc: func(_ *api.SupervisedSession) error {
				return errors.New("force add fail")
			},
			GetFunc: func(_ api.ID) (*api.SupervisedSession, bool) {
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
		ID:          api.ID(supervisorID),
		Name:        "default",
		LogFile:     "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
	}

	go func() {
		exitCh <- sc.Run(&spec)
	}()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSessionStore) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSessionStore, err)
	}
}
