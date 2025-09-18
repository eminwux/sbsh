package sessionrunner

import (
	"context"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
)

type SessionRunnerTest struct {
	OpenSocketCtrlFunc func(sessionID api.SessionID) (net.Listener, error)
	StartServerFunc    func(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error)
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
func (sr *SessionRunnerTest) StartServer(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
	if sr.StartServerFunc != nil {
		sr.StartServerFunc(ctx, ln, sc, readyCh, doneCh)
	}
}
