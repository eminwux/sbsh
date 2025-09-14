package session

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/session/session_runner"
	"testing"
	"time"
)

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

	newSessionRunner = func() session_runner.SessionRunner {
		return &session_runner.SessionRunnerTest{
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
