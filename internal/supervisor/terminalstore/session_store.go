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

package terminalstore

import (
	"errors"
	"sync"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type TerminalStore interface {
	Add(s *api.SupervisedTerminal) error
	Get(id api.ID) (*api.SupervisedTerminal, bool)
	ListLive() []api.ID
	Remove(id api.ID)
	Current() api.ID
	SetCurrent(id api.ID) error
}

type Exec struct {
	mu        sync.RWMutex
	terminals map[api.ID]*api.SupervisedTerminal
	current   api.ID
}

func NewTerminalStoreExec() TerminalStore {
	return &Exec{
		terminals: make(map[api.ID]*api.SupervisedTerminal),
	}
}

func NewSupervisedTerminal(spec *api.TerminalSpec) *api.SupervisedTerminal {
	return &api.SupervisedTerminal{
		Spec:        spec,
		Command:     spec.Command,
		CommandArgs: spec.CommandArgs,
	}
}

/* Basic ops */

func (m *Exec) Add(s *api.SupervisedTerminal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.terminals[s.Spec.ID] != nil {
		return errdefs.ErrTerminalExists
	}
	m.terminals[s.Spec.ID] = s
	if m.current == "" {
		m.current = s.Spec.ID
	}
	return nil
}

func (m *Exec) Get(id api.ID) (*api.SupervisedTerminal, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.terminals[id]
	return s, ok
}

func (m *Exec) ListLive() []api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]api.ID, 0, len(m.terminals))
	for id := range m.terminals {
		out = append(out, id)
	}
	return out
}

func (m *Exec) Remove(id api.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.terminals, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live terminal
	}
}

func (m *Exec) Current() api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *Exec) SetCurrent(id api.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.terminals[id]; !ok {
		return errors.New("unknown terminal id")
	}
	m.current = id
	return nil
}
