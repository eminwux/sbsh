package session_runner

import (
	"net"
	"sbsh/pkg/api"
)

type SessionRunnerTest struct {
	OpenSocketCtrlFunc func(sessionID api.SessionID) (net.Listener, error)
}

func NewSessionRunnerTest() SessionRunner {
	return &SessionRunnerTest{}
}

func (sr *SessionRunnerTest) OpenSocketCtrl(sessionID api.SessionID) (net.Listener, error) {
	if sr.OpenSocketCtrlFunc != nil {
		return sr.OpenSocketCtrlFunc(sessionID)
	}
	return nil, nil
}
