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
	"sync"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type TerminalStore interface {
	Add(s *api.AttachedTerminal) error
}

type Exec struct {
	mu        sync.Mutex
	terminals map[api.ID]*api.AttachedTerminal
}

func NewTerminalStoreExec() TerminalStore {
	return &Exec{
		terminals: make(map[api.ID]*api.AttachedTerminal),
	}
}

func NewSupervisedTerminal(spec *api.TerminalSpec) *api.AttachedTerminal {
	return &api.AttachedTerminal{
		Spec:        spec,
		Command:     spec.Command,
		CommandArgs: spec.CommandArgs,
	}
}

func (m *Exec) Add(s *api.AttachedTerminal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.terminals[s.Spec.ID] != nil {
		return errdefs.ErrTerminalExists
	}
	m.terminals[s.Spec.ID] = s
	return nil
}
