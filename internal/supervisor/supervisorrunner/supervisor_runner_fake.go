// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package supervisorrunner

import (
	"context"
	"errors"
	"net"

	"github.com/eminwux/sbsh/internal/supervisor/supervisorrpc"
	"github.com/eminwux/sbsh/pkg/api"
)

// ErrFuncNotSet is returned when a test function has not been stubbed.
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
	OpenSocketCtrlFunc func() error
	StartServerFunc    func(
		ctx context.Context,
		sc *supervisorrpc.SupervisorControllerRPC,
		readyCh chan error,
		doneCh chan error,
	)
	IDFunc              func() api.ID
	CloseFunc           func(reason error) error
	ResizeFunc          func(args api.ResizeArgs)
	AttachFunc          func(session *api.SupervisedSession) error
	CreateMetadataFunc  func() error
	DetachFunc          func() error
	StartSessionCmdFunc func(session *api.SupervisedSession) error
}

// NewSupervisorRunnerTest returns a new SupervisorRunnerTest instance.
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

func (t *SupervisorRunnerTest) StartServer(
	ctx context.Context,
	sc *supervisorrpc.SupervisorControllerRPC,
	readyCh chan error,
	doneCh chan error,
) {
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

func (t *SupervisorRunnerTest) Attach(session *api.SupervisedSession) error {
	if t.AttachFunc != nil {
		return t.AttachFunc(session)
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

func (t *SupervisorRunnerTest) StartSessionCmd(session *api.SupervisedSession) error {
	if t.StartSessionCmdFunc != nil {
		return t.StartSessionCmdFunc(session)
	}
	return ErrFuncNotSet
}
