package sessionrunner

import (
	"context"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/session/sessionrpc"
)

type SessionRunnerTest struct {
	OpenSocketCtrlFunc func() error
	StartServerFunc    func(sc *sessionrpc.SessionControllerRPC, readyCh chan error)
	StartSessionFunc   func(evCh chan<- SessionRunnerEvent) error
	CloseFunc          func(reason error) error
	ResizeFunc         func(args api.ResizeArgs)
	IDFunc             func() api.ID
	CreateMetadataFunc func() error
	AttachFunc         func(id *api.ID, response *api.ResponseWithFD) error
	DetachFunc         func(id *api.ID) error
}

func NewSessionRunnerTest(ctx context.Context) SessionRunner {
	return &SessionRunnerTest{}
}

func (sr *SessionRunnerTest) OpenSocketCtrl() error {
	if sr.OpenSocketCtrlFunc != nil {
		return sr.OpenSocketCtrlFunc()
	}
	return nil
}
func (sr *SessionRunnerTest) StartServer(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
	if sr.StartServerFunc != nil {
		sr.StartServerFunc(sc, readyCh)
	}
}

func (sr *SessionRunnerTest) StartSession(evCh chan<- SessionRunnerEvent) error {
	if sr.OpenSocketCtrlFunc != nil {
		return sr.StartSessionFunc(evCh)
	}
	return nil
}

func (sr *SessionRunnerTest) ID() api.ID {
	if sr.IDFunc != nil {
		return sr.IDFunc()
	}
	return api.ID("")
}

func (sr *SessionRunnerTest) Close(reason error) error {
	if sr.CloseFunc != nil {
		return sr.CloseFunc(reason)
	}
	return nil
}

func (sr *SessionRunnerTest) Resize(args api.ResizeArgs) {
	if sr.ResizeFunc != nil {
		sr.ResizeFunc(args)
	}
}

func (sr *SessionRunnerTest) CreateMetadata() error {
	if sr.CreateMetadataFunc != nil {
		sr.CreateMetadataFunc()
	}
	return nil
}

func (sr *SessionRunnerTest) Attach(id *api.ID, response *api.ResponseWithFD) error {
	if sr.AttachFunc != nil {
		return sr.AttachFunc(id, response)
	}
	return errdefs.ErrFuncNotSet
}

func (sr *SessionRunnerTest) Detach(id *api.ID) error {
	if sr.DetachFunc != nil {
		sr.DetachFunc(id)
	}
	return errdefs.ErrFuncNotSet
}
