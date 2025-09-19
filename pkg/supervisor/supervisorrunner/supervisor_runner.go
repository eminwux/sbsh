package supervisorrunner

import (
	"context"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor/supervisorrpc"
	"time"
)

type SupervisorRunner interface {
	OpenSocketCtrl() (net.Listener, error)
	StartServer(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error)
	StartSession(ctx context.Context, evCh chan<- SupervisorRunnerEvent) error
	ID() api.SessionID
	Close(reason error) error
	Resize(args api.ResizeArgs)
}

type SupervisorRunnerExec struct {
	sessionID        api.SessionID
	sessionCtxCancel context.CancelFunc
	sessionCtx       context.Context
}

type SupervisorRunnerEvent struct {
	ID    api.SessionID
	Type  SupervisorRunnerEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

type SupervisorRunnerEventType int

const (
	EvError SupervisorRunnerEventType = iota // abnormal error
	EvCmdExited
)

func NewSupervisorRunnerExec(spec *api.SessionSpec) SupervisorRunner {
	return &SupervisorRunnerExec{}
}

func (s *SupervisorRunnerExec) OpenSocketCtrl() (net.Listener, error) {
	// Returns a TCP listener bound to an ephemeral port
	return net.Listen("tcp", "127.0.0.1:0")
}

func (s *SupervisorRunnerExec) StartServer(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
	// Immediately signal that the server is ready
	select {
	case readyCh <- nil:
	default:
	}
	// No serving loop implemented here
}

func (s *SupervisorRunnerExec) StartSession(ctx context.Context, evCh chan<- SupervisorRunnerEvent) error {
	// No-op, just succeed
	return nil
}

func (s *SupervisorRunnerExec) ID() api.SessionID {
	return s.sessionID
}

func (s *SupervisorRunnerExec) Close(reason error) error {
	// No-op
	return nil
}

func (s *SupervisorRunnerExec) Resize(args api.ResizeArgs) {
	// No-op
}
