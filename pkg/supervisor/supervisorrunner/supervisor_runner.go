package supervisorrunner

import (
	"context"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
)

type SupervisorRunner interface {
	OpenSocketCtrl() error
	StartServer(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, doneCh chan error)
	StartSupervisor(ctx context.Context, evCh chan<- SupervisorRunnerEvent, session *sessionstore.SupervisedSession) error
	ID() api.ID
	Close(reason error) error
	Resize(args api.ResizeArgs)
	SetCurrentSession(id api.ID) error
	CreateMetadata() error
	Detach() error
}

func (s *SupervisorRunnerExec) ID() api.ID {
	return s.session.Id
}
