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

package terminal

import (
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type ControllerTest struct {
	Exit chan error

	AddedSpec *api.TerminalSpec

	RunFunc       func(spec *api.TerminalSpec) error
	WaitReadyFunc func() error
	WaitCloseFunc func() error
	PingFunc      func(in *api.PingMessage) (*api.PingMessage, error)
	CloseFunc     func(reason error) error
	ResizeFunc    func()
	AttachFunc    func(id *api.ID, response *api.ResponseWithFD) error
	DetachFunc    func(id *api.ID) error
	MetadataFunc  func() (*api.TerminalDoc, error)
	StateFunc     func() (*api.TerminalStatusMode, error)
}

func (f *ControllerTest) Run(spec *api.TerminalSpec) error {
	f.AddedSpec = spec
	if f.RunFunc != nil {
		return f.RunFunc(spec)
	}
	return errdefs.ErrFuncNotSet
}

func (f *ControllerTest) WaitReady() error {
	if f.WaitReadyFunc != nil {
		return f.WaitReadyFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (f *ControllerTest) WaitClose() error {
	if f.WaitCloseFunc != nil {
		return f.WaitCloseFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (f *ControllerTest) Ping(in *api.PingMessage) (*api.PingMessage, error) {
	if f.PingFunc != nil {
		return f.PingFunc(in)
	}
	return nil, errdefs.ErrFuncNotSet
}

func (f *ControllerTest) Close(reason error) error {
	if f.CloseFunc != nil {
		return f.CloseFunc(reason)
	}
	return errdefs.ErrFuncNotSet
}

func (f *ControllerTest) Resize(_ api.ResizeArgs) {
	if f.ResizeFunc != nil {
		f.ResizeFunc()
	}
}

func (f *ControllerTest) Attach(id *api.ID, response *api.ResponseWithFD) error {
	if f.AttachFunc != nil {
		return f.AttachFunc(id, response)
	}
	return errdefs.ErrFuncNotSet
}

func (f *ControllerTest) Detach(id *api.ID) error {
	if f.DetachFunc != nil {
		return f.DetachFunc(id)
	}
	return errdefs.ErrFuncNotSet
}

func (f *ControllerTest) Metadata() (*api.TerminalDoc, error) {
	if f.MetadataFunc != nil {
		return f.MetadataFunc()
	}
	return nil, errdefs.ErrFuncNotSet
}

func (f *ControllerTest) State() (*api.TerminalStatusMode, error) {
	if f.StateFunc != nil {
		return f.StateFunc()
	}
	return nil, errdefs.ErrFuncNotSet
}
