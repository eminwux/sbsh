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

// ////////////////////////////////////////////////////////
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

	sessionCtrl.AddSession(&spec)

	newSessionRunner = func() sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			OpenSocketCtrlFunc: func(sessionID api.SessionID) (net.Listener, error) {
				return newStubListener(), fmt.Errorf("error opening listener")
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	go sessionCtrl.Run()

	time.Sleep(100 * time.Millisecond)

	message := "could not open control socket"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}

func Test_StartServerError(t *testing.T) {
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

	sessionCtrl.AddSession(&spec)

	newSessionRunner = func() sessionrunner.SessionRunner {
		return &sessionrunner.SessionRunnerTest{
			OpenSocketCtrlFunc: func(sessionID api.SessionID) (net.Listener, error) {
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

	go sessionCtrl.Run()

	time.Sleep(100 * time.Millisecond)

	message := "failed to start server"
	if !bytes.Contains(buf.Bytes(), []byte(message)) {
		t.Fatalf("expected '"+message+"' in logs; got: %s", buf.String())
	}

}
