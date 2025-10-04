package sessionrunner

import (
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
)

type SessionRunner interface {
	OpenSocketCtrl() error
	StartServer(sc *sessionrpc.SessionControllerRPC, readyCh chan error)
	StartSession(evCh chan<- SessionRunnerEvent) error
	ID() api.ID
	Close(reason error) error
	Resize(args api.ResizeArgs)
	CreateMetadata() error
	Detach() error
	Attach(id *api.ID, response *api.ResponseWithFD) error
}
