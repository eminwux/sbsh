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

package sessionstore

import (
	"errors"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type Test struct {
	// Last-call trackers (useful for assertions)
	LastAdded        *api.SupervisedSession
	LastGetID        api.ID
	LastRemovedID    api.ID
	LastSetCurrentID api.ID

	// Optional: store a value to be returned by Current() when CurrentFunc is nil
	CurrentID api.ID

	// Stub functions (set these in tests to control behavior)
	AddFunc        func(s *api.SupervisedSession) error
	GetFunc        func(id api.ID) (*api.SupervisedSession, bool)
	ListLiveFunc   func() []api.ID
	RemoveFunc     func(id api.ID)
	CurrentFunc    func() api.ID
	SetCurrentFunc func(id api.ID) error
}

func NewSessionStoreTest() *Test {
	return &Test{
		AddFunc: func(s *api.SupervisedSession) error {
			if s == nil {
				return errors.New("nil session")
			}
			return nil
		},
		GetFunc: func(id api.ID) (*api.SupervisedSession, bool) {
			if id == "" {
				return nil, false
			}
			return &api.SupervisedSession{}, true
		},
		ListLiveFunc: func() []api.ID {
			return []api.ID{"sess-1", "sess-2"}
		},
		RemoveFunc: func(_ api.ID) {
			// no-op, LastRemovedID is tracked automatically
		},
		CurrentFunc: func() api.ID {
			return "sess-1"
		},
		SetCurrentFunc: func(id api.ID) error {
			if id == "" {
				return errors.New("cannot set empty id")
			}
			return nil
		},
	}
}

func (t *Test) Add(s *api.SupervisedSession) error {
	t.LastAdded = s
	if t.AddFunc != nil {
		return t.AddFunc(s)
	}
	return errdefs.ErrFuncNotSet
}

func (t *Test) Get(id api.ID) (*api.SupervisedSession, bool) {
	t.LastGetID = id
	if t.GetFunc != nil {
		return t.GetFunc(id)
	}
	return nil, false
}

func (t *Test) ListLive() []api.ID {
	if t.ListLiveFunc != nil {
		return t.ListLiveFunc()
	}
	return nil
}

func (t *Test) Remove(id api.ID) {
	t.LastRemovedID = id
	if t.RemoveFunc != nil {
		t.RemoveFunc(id)
	}
}

func (t *Test) Current() api.ID {
	if t.CurrentFunc != nil {
		return t.CurrentFunc()
	}
	return t.CurrentID
}

func (t *Test) SetCurrent(id api.ID) error {
	t.LastSetCurrentID = id
	if t.SetCurrentFunc != nil {
		return t.SetCurrentFunc(id)
	}
	return errdefs.ErrFuncNotSet
}
