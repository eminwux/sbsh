package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/session/sessionrpc"
	"sbsh/pkg/session/sessionrunner"
	"testing"
	"time"
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

func Test_ErrSpecCmdMissing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
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

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSpecCmdMissing) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSpecCmdMissing, err)
	}

}

func Test_ErrOpenSocketCtrl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return api.ID("iajs099")
			},
			OpenSocketCtrlFunc: func() error {
				return fmt.Errorf("error opening listener")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}

}

func Test_ErrStartRPCServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- fmt.Errorf("make server fail")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartRPCServer, err)
	}

}

func Test_ErrStartSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(evCh chan<- sessionrunner.SessionRunnerEvent) error {
				return fmt.Errorf("make start session fail")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh = make(chan error, 1)

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
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
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

	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	ctrlReady = make(chan struct{}, 1)
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh = make(chan error, 1)

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
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
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

	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	ctrlReady = make(chan struct{}, 1)
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh = make(chan error, 1)

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	rpcDoneCh <- fmt.Errorf("make rpc server exit with error")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}

}

func Test_WaitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
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

	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	ctrlReady = make(chan struct{}, 1)
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh = make(chan error, 1)

	readyReturn := make(chan error)

	go func(chan error) {
		readyReturn <- sessionCtrl.WaitReady()
	}(readyReturn)

	exitCh := make(chan error)
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
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
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

	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	ctrlReady = make(chan struct{}, 1)
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh = make(chan error, 1)

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	ev := sessionrunner.SessionRunnerEvent{
		ID:   spec.ID,
		Type: sessionrunner.EvCmdExited,
		Err:  fmt.Errorf("session has been closed"),
		When: time.Now(),
	}

	eventsCh <- ev

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}
}

func Test_HandleEvent_EvError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.ID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(ctx context.Context, spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.ID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() error {
				return nil
			},
			StartServerFunc: func(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
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

	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	ctrlReady = make(chan struct{})
	eventsCh = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh = make(chan error, 1)

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	ev := sessionrunner.SessionRunnerEvent{
		ID:   spec.ID,
		Type: sessionrunner.EvError,
		Err:  fmt.Errorf("session has been closed"),
		When: time.Now(),
	}

	eventsCh <- ev

	// Respawn implementation pending
}
