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
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/session/sessionrpc"
	"github.com/eminwux/sbsh/internal/session/sessionrunner"
	"github.com/eminwux/sbsh/pkg/api"
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

// use in test.
func newStubListener() net.Listener { return &fakeListener{} }

func Test_ErrSpecCmdMissing(t *testing.T) {
	sessionCtrl := NewSessionController(context.Background())

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)

	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSpecCmdMissing) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSpecCmdMissing, err)
	}
}

func Test_ErrOpenSocketCtrl(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
				return errors.New("error opening listener")
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)

	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}
}

func Test_ErrStartRPCServer(t *testing.T) {
	sessionCtrl := NewSessionController(context.Background())

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- errors.New("make server fail")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	exitCh := make(chan error)
	defer close(exitCh)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartRPCServer, err)
	}
}

func Test_ErrStartSession(t *testing.T) {
	sessionCtrl := NewSessionController(context.Background())

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
				return errors.New("make start session fail")
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)

	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartSession) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartSession, err)
	}
}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)

	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	time.Sleep(500 * time.Microsecond)

	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
	}
}

func Test_ErrRPCServerExited(t *testing.T) {
	sessionCtrl := NewSessionController(context.Background())

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	rpcDoneCh <- errors.New("make rpc server exit with error")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}
}

func Test_WaitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	exitCh := make(chan error)

	readyReturn := make(chan error)

	go func(chan error) {
		readyReturn <- sessionCtrl.WaitReady()
	}(readyReturn)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-readyReturn; err != nil {
		t.Fatalf("expected 'nil'; got: '%v'", err)
	}
	cancel()
	<-exitCh
}

func Test_HandleEvent_EvCmdExited(t *testing.T) {
	sessionCtrl := NewSessionController(context.Background())

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	exitCh := make(chan error)

	readyReturn := make(chan error)
	defer close(readyReturn)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	ev := sessionrunner.SessionRunnerEvent{
		ID:   spec.ID,
		Type: sessionrunner.EvCmdExited,
		Err:  errors.New("session has been closed"),
		When: time.Now(),
	}

	eventsCh <- ev

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_HandleEvent_EvError(t *testing.T) {
	sessionCtrl := NewSessionController(context.Background())

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessionLocal,
		Name:        "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	exitCh := make(chan error)

	readyReturn := make(chan error)
	defer close(readyReturn)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	ev := sessionrunner.SessionRunnerEvent{
		ID:   spec.ID,
		Type: sessionrunner.EvError,
		Err:  errors.New("session has been closed"),
		When: time.Now(),
	}

	eventsCh <- ev
}
