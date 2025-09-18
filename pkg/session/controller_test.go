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

func Test_EmptySpecCmd(t *testing.T) {
	exitCh := make(chan error)
	sessionCtrl := NewSessionController(context.Background(), exitCh)

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

	go sessionCtrl.Run(&spec)

	time.Sleep(100 * time.Millisecond)

	message := "empty command in SessionSpec"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}

func Test_OpenSocketCtrlError(t *testing.T) {
	exitCh := make(chan error)
	sessionCtrl := NewSessionController(context.Background(), exitCh)

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

	go sessionCtrl.Run(&spec)

	time.Sleep(100 * time.Millisecond)

	message := "could not open control socket"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}

func Test_RPCServerNotReady(t *testing.T) {
	exitCh := make(chan error)
	sessionCtrl := NewSessionController(context.Background(), exitCh)

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
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- fmt.Errorf("make server fail")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	go sessionCtrl.Run(&spec)

	time.Sleep(100 * time.Millisecond)

	message := "failed to start server"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}

func Test_StartSessionFailed(t *testing.T) {
	exitCh := make(chan error)
	sessionCtrl := NewSessionController(context.Background(), exitCh)

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
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- api.SessionEvent) error {
				return fmt.Errorf("make start session fail")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	go sessionCtrl.Run(&spec)

	time.Sleep(100 * time.Millisecond)

	message := "failed to start session"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}

func Test_ContextDone(t *testing.T) {
	exitCh := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, exitCh)

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
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- api.SessionEvent) error {
				return nil
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	go sessionCtrl.Run(&spec)

	time.Sleep(10 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)

	message := "parent context channel has been closed"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}

func Test_RPCServerDone(t *testing.T) {
	exitCh := make(chan error)
	ctx, _ := context.WithCancel(context.Background())
	sessionCtrl := NewSessionController(ctx, exitCh)

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
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
				readyCh <- nil
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- api.SessionEvent) error {
				return nil
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	rpcDoneCh = make(chan error)
	go sessionCtrl.Run(&spec)

	time.Sleep(10 * time.Millisecond)
	rpcDoneCh <- fmt.Errorf("make rpc server exit with error")
	time.Sleep(10 * time.Millisecond)

	message := "rpc server has failed"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}
