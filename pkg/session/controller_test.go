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
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return api.SessionID("iajs099")
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), fmt.Errorf("error opening listener")
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

	if err := <-exitCh; err != nil && !errors.Is(err, ErrSpecCmdMissing) {
		t.Fatalf("expected '%v'; got: '%v'", ErrSpecCmdMissing, err)
	}

}

func Test_ErrOpenSocketCtrl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return api.SessionID("iajs099")
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), fmt.Errorf("error opening listener")
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

	if err := <-exitCh; err != nil && !errors.Is(err, ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", ErrOpenSocketCtrl, err)
	}

}

func Test_ErrStartRPCServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
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

	if err := <-exitCh; err != nil && !errors.Is(err, ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", ErrStartRPCServer, err)
	}

}

func Test_ErrStartSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- sessionrunner.SessionRunnerEvent) error {
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

	if err := <-exitCh; err != nil && !errors.Is(err, ErrStartSession) {
		t.Fatalf("expected '%v'; got: '%v'", ErrStartSession, err)
	}

}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- sessionrunner.SessionRunnerEvent) error {
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

	time.Sleep(500 * time.Microsecond)

	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", ErrContextDone, err)
	}

}

func Test_ErrRPCServerExited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- sessionrunner.SessionRunnerEvent) error {
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

	rpcDoneCh <- fmt.Errorf("make rpc server exit with error")

	if err := <-exitCh; err != nil && !errors.Is(err, ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", ErrRPCServerExited, err)
	}

}

func Test_WaitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- sessionrunner.SessionRunnerEvent) error {
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

	readyReturn := make(chan error)

	go func(chan error) {
		readyReturn <- sessionCtrl.WaitReady()
	}(readyReturn)

	ctrlReady <- struct{}{}

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
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
		Type: sessionrunner.EvCmdExited,
		Err:  fmt.Errorf("session has been closed"),
		When: time.Now(),
	}

	eventsCh <- ev

	if err := <-exitCh; err != nil && !errors.Is(err, ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", ErrCloseReq, err)
	}
}

func Test_HandleEvent_EvError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, cancel)

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID("abcdef"),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     "/bin/bash",
		CommandArgs: nil,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	newSessionRunner = func(spec *api.SessionSpec) sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			IDFunc: func() api.SessionID {
				return spec.ID
			},
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				return newStubListener(), nil
			},
			StartServerFunc: func(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- sessionrunner.SessionRunnerEvent) error {
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
