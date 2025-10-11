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
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				return errors.New("force socket fail")
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			IDFunc:             func() api.ID { return "" },
			CloseFunc:          func(reason error) error { return nil },
			ResizeFunc:         func(args api.ResizeArgs) {},
			CreateMetadataFunc: func() error { return nil },
		}
	}

	supervisorID := naming.RandomID()
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Name:    "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
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
	// ctx, cancel := context.WithCancel(context.Background())
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
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
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
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
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	readyReturn := make(chan error)
	go func(chan error) {
		readyReturn <- sc.WaitReady()
	}(readyReturn)

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sc.Run(&spec)
	}(exitCh)

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
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return errors.New("force session start fail")
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartSessionCmdFunc: func(session *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return errors.New("force add fail")
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
		Kind:    api.AttachToSession,
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sc.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrAttach) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrAttach, err)
	}
}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sc := NewSupervisorController(ctx).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartSessionCmdFunc: func(session *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
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
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartSessionCmdFunc: func(session *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
	}
	go func(exitCh chan error) {
		exitCh <- sc.Run(&spec)
	}(exitCh)

	<-sc.ctrlReadyCh
	sc.rpcDoneCh <- errors.New("force rpc server exit")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}
}

func Test_ErrSessionExists(t *testing.T) {
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartSessionCmdFunc: func(session *api.SupervisedSession) error {
				return nil
			},
		}
	}
	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
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
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartSessionCmdFunc: func(session *api.SupervisedSession) error {
				return nil
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
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
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
			StartSessionCmdFunc: func(session *api.SupervisedSession) error {
				return errors.New("force cmd start fail")
			},
		}
	}

	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return nil
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
	}

	go func(exitCh chan error) {
		exitCh <- sc.Run(&spec)
	}(exitCh)

	//	<-ctrlReady

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartSessionCmd) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartSessionCmd, err)
	}
}

func Test_ErrSessionStore(t *testing.T) {
	sc := NewSupervisorController(context.Background()).(*SupervisorController)
	sc.NewSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(session *api.SupervisedSession) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
			CreateMetadataFunc: func() error {
				return nil
			},
		}
	}
	sc.NewSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return errors.New("force add fail")
			},
			GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
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
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
		SessionSpec: &api.SessionSpec{},
	}

	go func(exitCh chan error) {
		exitCh <- sc.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSessionStore) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSessionStore, err)
	}
}
