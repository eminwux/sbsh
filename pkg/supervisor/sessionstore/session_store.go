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
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sync"
)

type SessionStore interface {
	Add(s *api.SupervisedSession) error
	Get(id api.ID) (*api.SupervisedSession, bool)
	ListLive() []api.ID
	Remove(id api.ID)
	Current() api.ID
	SetCurrent(id api.ID) error
}

type SessionStoreExec struct {
	mu       sync.RWMutex
	sessions map[api.ID]*api.SupervisedSession
	current  api.ID
}

func NewSessionStoreExec() SessionStore {
	return &SessionStoreExec{
		sessions: make(map[api.ID]*api.SupervisedSession),
	}
}

func NewSupervisedSession(spec *api.SessionSpec) *api.SupervisedSession {
	return &api.SupervisedSession{
		Id:          spec.ID,
		Kind:        spec.Kind,
		Name:        spec.Name,
		Command:     spec.Command,
		CommandArgs: spec.CommandArgs,
		Env:         spec.Env,
		LogFilename: spec.LogFilename,
		SocketCtrl:  spec.SockerCtrl,
		SocketIO:    spec.SocketIO,
	}
}

/* Basic ops */

func (m *SessionStoreExec) Add(s *api.SupervisedSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[s.Id] != nil {
		return errdefs.ErrSessionExists
	}
	m.sessions[s.Id] = s
	if m.current == "" {
		m.current = s.Id
	}
	return nil
}

func (m *SessionStoreExec) Get(id api.ID) (*api.SupervisedSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionStoreExec) ListLive() []api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]api.ID, 0, len(m.sessions))
	for id := range m.sessions {
		out = append(out, id)
	}
	return out
}

func (m *SessionStoreExec) Remove(id api.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live session
	}
}

func (m *SessionStoreExec) Current() api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *SessionStoreExec) SetCurrent(id api.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	}
	m.current = id
	return nil
}
