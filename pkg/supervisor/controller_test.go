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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/naming"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
	"sbsh/pkg/supervisor/supervisorrunner"
)

type fakeListener struct{}

func (f *fakeListener) Accept() (net.Conn, error) {
	// No real conns; make it obvious if code tries to use it
	return nil, errors.New("stub listener: Accept not implemented")
}
func (f *fakeListener) Close() error { return nil }
func (f *fakeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// use in test
func newStubListener() net.Listener { return &fakeListener{} }

func Test_ErrOpenSocketCtrl(t *testing.T) {
	// ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return fmt.Errorf("force socket fail")
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})

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
		readyReturn <- sessionCtrl.WaitReady()
	}(readyReturn)

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	select {
	case err := <-readyReturn:
		if err != nil {
			t.Fatalf("expected 'nil'; got: '%v'", err)
		}
	case <-time.After(100 * time.Millisecond): // pick a sensible deadline
		t.Fatalf("WaitReady timed out")
	}

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}
}

func Test_ErrStartRPCServer(t *testing.T) {
	// ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() error {
				// default: return nil listener and nil error
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- fmt.Errorf("force server fail"):
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})

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
		readyReturn <- sessionCtrl.WaitReady()
	}(readyReturn)

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
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
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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
				return fmt.Errorf("force session start fail")
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

	newSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return fmt.Errorf("force add fail")
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})

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
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrAttach) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrAttach, err)
	}
}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(ctx)

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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

	newSessionStore = func() sessionstore.SessionStore {
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
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})

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
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	time.Sleep(10 * time.Millisecond)
	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
	}
}

func Test_ErrRPCServerExited(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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

	newSessionStore = func() sessionstore.SessionStore {
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})
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
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	rpcDoneCh <- fmt.Errorf("force rpc server exit")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}
}

func Test_ErrSessionExists(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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
	newSessionStore = func() sessionstore.SessionStore {
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})
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
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	closeReqCh <- fmt.Errorf("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_ErrCloseReq(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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

	newSessionStore = func() sessionstore.SessionStore {
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})
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
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	closeReqCh <- fmt.Errorf("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_ErrStartSessionCmd(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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
				return fmt.Errorf("force cmd start fail")
			},
		}
	}

	newSessionStore = func() sessionstore.SessionStore {
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})
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
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartSessionCmd) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartSessionCmd, err)
	}
}

func Test_ErrSessionStore(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner {
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
	newSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *api.SupervisedSession) error {
				return fmt.Errorf("force add fail")
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

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{})
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
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSessionStore) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSessionStore, err)
	}
}
