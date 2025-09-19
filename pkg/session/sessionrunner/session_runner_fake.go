package sessionrunner

import (
	"context"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
)

type SessionRunnerTest struct {
	OpenSocketCtrlFunc func() (net.Listener, error)
	StartServerFunc    func(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error)
	StartSessionFunc   func(ctx context.Context, evCh chan<- SessionRunnerEvent) error
	CloseFunc          func()
	ResizeFunc         func(args api.ResizeArgs)
	IDFunc             func() api.SessionID
}

func NewSessionRunnerTest() SessionRunner {
	return &SessionRunnerTest{}
}

func (sr *SessionRunnerTest) OpenSocketCtrl() (net.Listener, error) {
	if sr.OpenSocketCtrlFunc != nil {
		return sr.OpenSocketCtrlFunc()
	}
	return nil, nil
}
func (sr *SessionRunnerTest) StartServer(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
	if sr.StartServerFunc != nil {
		sr.StartServerFunc(ctx, ln, sc, readyCh, doneCh)
	}
}

func (sr *SessionRunnerTest) StartSession(ctx context.Context, evCh chan<- SessionRunnerEvent) error {
	if sr.OpenSocketCtrlFunc != nil {
		return sr.StartSessionFunc(ctx, evCh)
	}
	return nil
}

func (sr *SessionRunnerTest) ID() api.SessionID {
	if sr.IDFunc != nil {
		return sr.IDFunc()
	}
	return api.SessionID("")
}

func (sr *SessionRunnerTest) Close() {
	if sr.CloseFunc != nil {
		sr.CloseFunc()
	}
}

func (sr *SessionRunnerTest) Resize(args api.ResizeArgs) {
	if sr.ResizeFunc != nil {
		sr.ResizeFunc(args)
	}
}
