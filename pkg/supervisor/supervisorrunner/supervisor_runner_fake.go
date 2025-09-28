package supervisorrunner

import (
	"context"
	"errors"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
)

// ErrFuncNotSet is returned when a test function has not been stubbed
var ErrFuncNotSet = errors.New("test function not set")

// SupervisorRunnerTest is a test double for SupervisorRunner
// It allows overriding behavior with function fields and
// capturing arguments for assertions in unit tests.
type SupervisorRunnerTest struct {
	Ctx context.Context
	// Last-call trackers
	LastListener   net.Listener
	LastController *supervisorrpc.SupervisorControllerRPC
	LastCtx        context.Context
	LastReason     error
	LastResize     api.ResizeArgs

	// Stub functions
	OpenSocketCtrlFunc    func() error
	StartServerFunc       func(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, doneCh chan error)
	IDFunc                func() api.ID
	CloseFunc             func(reason error) error
	ResizeFunc            func(args api.ResizeArgs)
	SetCurrentSessionFunc func(id api.ID) error
	StartSupervisorFunc   func(ctx context.Context, evCh chan<- SupervisorRunnerEvent) error
	CreateMetadataFunc    func() error
	DetachFunc            func() error
}

// NewSupervisorRunnerTest returns a new SupervisorRunnerTest instance
func NewSupervisorRunnerTest(ctx context.Context, spec *api.SupervisorSpec) *SupervisorRunnerTest {
	return &SupervisorRunnerTest{
		Ctx: ctx,
	}
}

func (t *SupervisorRunnerTest) OpenSocketCtrl() error {
	if t.OpenSocketCtrlFunc != nil {
		return t.OpenSocketCtrlFunc()
	}
	return ErrFuncNotSet
}

func (t *SupervisorRunnerTest) StartServer(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, doneCh chan error) {
	t.LastCtx = ctx
	t.LastController = sc
	if t.StartServerFunc != nil {
		t.StartServerFunc(ctx, sc, readyCh, doneCh)
	}
}

func (t *SupervisorRunnerTest) ID() api.ID {
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
func (t *SupervisorRunnerTest) SetCurrentSession(id api.ID) error {
	if t.SetCurrentSessionFunc != nil {
		return t.SetCurrentSessionFunc(id)
	}
	return ErrFuncNotSet
}

func (t *SupervisorRunnerTest) StartSupervisor(ctx context.Context, evCh chan<- SupervisorRunnerEvent, session *sessionstore.SupervisedSession) error {
	if t.StartSupervisorFunc != nil {
		return t.StartSupervisorFunc(ctx, evCh)
	}
	return ErrFuncNotSet
}
func (t *SupervisorRunnerTest) CreateMetadata() error {
	if t.CreateMetadataFunc != nil {
		return t.CreateMetadataFunc()
	}
	return ErrFuncNotSet
}
func (t *SupervisorRunnerTest) Detach() error {
	if t.DetachFunc != nil {
		return t.DetachFunc()
	}
	return ErrFuncNotSet
}
