package session_runner

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
)

type SessionRunner interface {
	OpenSocketCtrl(sessionID api.SessionID) (net.Listener, error)
}

type SessionRunnerExec struct {
}

func NewSessionRunnerExec() SessionRunner {
	return &SessionRunnerExec{}
}

func (sr *SessionRunnerExec) OpenSocketCtrl(sessionID api.SessionID) (net.Listener, error) {

	// Set up sockets
	base, err := common.RuntimeBaseSessions()
	if err != nil {
		return nil, err
	}

	runPath := filepath.Join(base, string(sessionID))
	if err := os.MkdirAll(runPath, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir session dir: %w", err)
	}

	socketCtrl := filepath.Join(runPath, "ctrl.sock")
	log.Printf("[sessionCtrl] CTRL socket: %s", socketCtrl)

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(socketCtrl); err != nil {
		log.Printf("[sessionCtrl] couldn't remove stale CTRL socket: %s\r\n", socketCtrl)
	}

	// Listen to CONTROL SOCKET
	ctrlLn, err := net.Listen("unix", socketCtrl)
	if err != nil {
		return nil, fmt.Errorf("listen ctrl: %w", err)
	}
	if err := os.Chmod(socketCtrl, 0o600); err != nil {
		ctrlLn.Close()
		return nil, err
	}

	// keep references for Close()

	return ctrlLn, nil
}
