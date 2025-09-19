package supervisorrunner

import (
	"context"
	"errors"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor/supervisorrpc"
)

// ErrFuncNotSet is returned when a test function has not been stubbed
var ErrFuncNotSet = errors.New("test function not set")

// SupervisorRunnerEvent is assumed to be defined elsewhere in your codebase
// type SupervisorRunnerEvent struct { ... }

// SupervisorRunnerTest is a test double for SupervisorRunner
// It allows overriding behavior with function fields and
// capturing arguments for assertions in unit tests.
type SupervisorRunnerTest struct {
	// Last-call trackers
	LastListener   net.Listener
	LastController *supervisorrpc.SupervisorControllerRPC
	LastCtx        context.Context
	LastReason     error
	LastResize     api.ResizeArgs

	// Stub functions
	OpenSocketCtrlFunc func() (net.Listener, error)
	StartServerFunc    func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error)
	StartSessionFunc   func(ctx context.Context, evCh chan<- SupervisorRunnerEvent) error
	IDFunc             func() api.SessionID
	CloseFunc          func(reason error) error
	ResizeFunc         func(args api.ResizeArgs)
}

// NewSupervisorRunnerTest returns a new SupervisorRunnerTest instance
func NewSupervisorRunnerTest() *SupervisorRunnerTest {
	return &SupervisorRunnerTest{}
}

func (t *SupervisorRunnerTest) OpenSocketCtrl() (net.Listener, error) {
	if t.OpenSocketCtrlFunc != nil {
		return t.OpenSocketCtrlFunc()
	}
	return nil, ErrFuncNotSet
}

func (t *SupervisorRunnerTest) StartServer(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
	t.LastCtx = ctx
	t.LastListener = ln
	t.LastController = sc
	if t.StartServerFunc != nil {
		t.StartServerFunc(ctx, ln, sc, readyCh, errCh)
	}
}

func (t *SupervisorRunnerTest) StartSession(ctx context.Context, evCh chan<- SupervisorRunnerEvent) error {
	t.LastCtx = ctx
	if t.StartSessionFunc != nil {
		return t.StartSessionFunc(ctx, evCh)
	}
	return ErrFuncNotSet
}

func (t *SupervisorRunnerTest) ID() api.SessionID {
	if t.IDFunc != nil {
		return t.IDFunc()
	}
	return "" // default empty ID if not set
}

func (t *SupervisorRunnerTest) Close(reason error) error {
	t.LastReason = reason
	if t.CloseFunc != nil {
		return t.CloseFunc(reason)
	}
	return ErrFuncNotSet
}

func (t *SupervisorRunnerTest) Resize(args api.ResizeArgs) {
	t.LastResize = args
	if t.ResizeFunc != nil {
		t.ResizeFunc(args)
	}
}
