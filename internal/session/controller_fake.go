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

package session

import (
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type FakeSessionController struct {
	Exit chan error

	AddedSpec *api.SessionSpec

	RunFunc       func(spec *api.SessionSpec) error
	WaitReadyFunc func() error
	WaitCloseFunc func() error
	StatusFunc    func() string
	CloseFunc     func(reason error) error
	ResizeFunc    func()
	AttachFunc    func(id *api.ID, response *api.ResponseWithFD) error
	DetachFunc    func(id *api.ID) error
}

func (f *FakeSessionController) Run(spec *api.SessionSpec) error {
	f.AddedSpec = spec
	if f.RunFunc != nil {
		return f.RunFunc(spec)
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) WaitReady() error {
	if f.WaitReadyFunc != nil {
		return f.WaitReadyFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) WaitClose() error {
	if f.WaitCloseFunc != nil {
		return f.WaitCloseFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) Status() string {
	if f.StatusFunc != nil {
		return f.StatusFunc()
	}
	return ""
}

func (f *FakeSessionController) Close(reason error) error {
	if f.CloseFunc != nil {
		return f.CloseFunc(reason)
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) Resize(_ api.ResizeArgs) {
	if f.ResizeFunc != nil {
		f.ResizeFunc()
	}
}

func (f *FakeSessionController) Attach(id *api.ID, response *api.ResponseWithFD) error {
	if f.AttachFunc != nil {
		return f.AttachFunc(id, response)
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) Detach(id *api.ID) error {
	if f.DetachFunc != nil {
		return f.DetachFunc(id)
	}
	return errdefs.ErrFuncNotSet
}
