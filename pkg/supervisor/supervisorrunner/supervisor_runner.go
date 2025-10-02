package supervisorrunner

import (
	"context"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor/supervisorrpc"
)

type SupervisorRunner interface {
	OpenSocketCtrl() error
	StartServer(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, doneCh chan error)
	Attach(session *api.SupervisedSession) error
	ID() api.ID
	Close(reason error) error
	Resize(args api.ResizeArgs)
	SetCurrentSession(id api.ID) error
	CreateMetadata() error
	Detach() error
	StartSessionCmd(session *api.SupervisedSession) error
}

func (sr *SupervisorRunnerExec) ID() api.ID {
	return sr.session.Id
}
