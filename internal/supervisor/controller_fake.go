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

package supervisor

import (
	"context"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

// ControllerTest is a test double for SupervisorController.
// It lets you override behavior with function fields and capture args.
type ControllerTest struct {
	// Last-call trackers (useful for assertions)
	LastCtx context.Context
	LastID  api.ID

	// Stub functions (set these in tests)
	RunFunc       func(spec *api.SupervisorSpec) error
	WaitReadyFunc func() error
	StartFunc     func() error
	CloseFunc     func(reason error) error
	WaitCloseFunc func() error
	DetachFunc    func() error
}

func NewSupervisorControllerTest() *ControllerTest {
	return &ControllerTest{
		RunFunc: func(_ *api.SupervisorSpec) error {
			// default: succeed without doing anything
			return nil
		},
		WaitReadyFunc: func() error {
			// default: succeed immediately
			return nil
		},
		StartFunc: func() error {
			// default: succeed immediately
			return nil
		},
	}
}

func (t *ControllerTest) Run(spec *api.SupervisorSpec) error {
	if t.RunFunc != nil {
		return t.RunFunc(spec)
	}
	return errdefs.ErrFuncNotSet
}

func (t *ControllerTest) WaitReady() error {
	if t.WaitReadyFunc != nil {
		return t.WaitReadyFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (t *ControllerTest) Start() error {
	if t.StartFunc != nil {
		return t.StartFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (t *ControllerTest) Close(reason error) error {
	if t.CloseFunc != nil {
		return t.CloseFunc(reason)
	}
	return errdefs.ErrFuncNotSet
}

func (t *ControllerTest) WaitClose() error {
	if t.WaitCloseFunc != nil {
		return t.WaitCloseFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (t *ControllerTest) Detach() error {
	if t.DetachFunc != nil {
		return t.DetachFunc()
	}
	return errdefs.ErrFuncNotSet
}
