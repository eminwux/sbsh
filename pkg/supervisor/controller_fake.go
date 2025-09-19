// /*
// Copyright © 2025 Emiliano Spinella (eminwux)
// */

package supervisor

import (
	"context"
	"sbsh/pkg/api"
)

// ErrFuncNotSet is returned when a test function has not been stubbed.
var ErrFuncNotSet error = nil

// SupervisorControllerTest is a test double for SupervisorController.
// It lets you override behavior with function fields and capture args.
type SupervisorControllerTest struct {
	// Last-call trackers (useful for assertions)
	LastCtx context.Context
	LastID  api.SessionID

	// Stub functions (set these in tests)
	RunFunc             func(ctx context.Context) error
	WaitReadyFunc       func(ctx context.Context) error
	SetCurrentSessionFn func(id api.SessionID) error
	StartFunc           func() error
}

func (t *SupervisorControllerTest) Run(ctx context.Context) error {
	t.LastCtx = ctx
	if t.RunFunc != nil {
		return t.RunFunc(ctx)
	}
	return ErrFuncNotSet
}

func (t *SupervisorControllerTest) WaitReady(ctx context.Context) error {
	if t.WaitReadyFunc != nil {
		return t.WaitReadyFunc(ctx)
	}
	return ErrFuncNotSet
}

func (t *SupervisorControllerTest) SetCurrentSession(id api.SessionID) error {
	t.LastID = id
	if t.SetCurrentSessionFn != nil {
		return t.SetCurrentSessionFn(id)
	}
	return ErrFuncNotSet
}

func (t *SupervisorControllerTest) Start() error {
	if t.StartFunc != nil {
		return t.StartFunc()
	}
	return ErrFuncNotSet
}

// (Optional) constructor with zeroed fields.
func NewSupervisorControllerTest() *SupervisorControllerTest {
	return &SupervisorControllerTest{
		RunFunc: func(ctx context.Context) error {
			// default: succeed without doing anything
			return nil
		},
		WaitReadyFunc: func(ctx context.Context) error {
			// default: succeed immediately
			return nil
		},
		SetCurrentSessionFn: func(id api.SessionID) error {
			// default: just accept the ID
			return nil
		},
		StartFunc: func() error {
			// default: succeed immediately
			return nil
		},
	}
}
