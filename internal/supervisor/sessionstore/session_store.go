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
	"sync"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type SessionStore interface {
	Add(s *api.SupervisedSession) error
	Get(id api.ID) (*api.SupervisedSession, bool)
	ListLive() []api.ID
	Remove(id api.ID)
	Current() api.ID
	SetCurrent(id api.ID) error
}

type Exec struct {
	mu       sync.RWMutex
	sessions map[api.ID]*api.SupervisedSession
	current  api.ID
}

func NewSessionStoreExec() SessionStore {
	return &Exec{
		sessions: make(map[api.ID]*api.SupervisedSession),
	}
}

func NewSupervisedSession(spec *api.SessionSpec) *api.SupervisedSession {
	return &api.SupervisedSession{
		ID:          spec.ID,
		Kind:        spec.Kind,
		Name:        spec.Name,
		Command:     spec.Command,
		CommandArgs: spec.CommandArgs,
		EnvInherit:  spec.EnvInherit,
		Env:         spec.Env,
		LogFile:     spec.LogFile,
		SocketFile:  spec.SocketFile,
		Prompt:      spec.Prompt,
	}
}

/* Basic ops */

func (m *Exec) Add(s *api.SupervisedSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[s.ID] != nil {
		return errdefs.ErrSessionExists
	}
	m.sessions[s.ID] = s
	if m.current == "" {
		m.current = s.ID
	}
	return nil
}

func (m *Exec) Get(id api.ID) (*api.SupervisedSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Exec) ListLive() []api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]api.ID, 0, len(m.sessions))
	for id := range m.sessions {
		out = append(out, id)
	}
	return out
}

func (m *Exec) Remove(id api.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live session
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
	if _, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	}
	m.current = id
	return nil
}
