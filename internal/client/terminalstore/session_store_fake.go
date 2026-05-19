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

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

type Test struct {
	LastAdded *api.AttachedTerminal

	AddFunc func(s *api.AttachedTerminal) error
}

func NewTerminalStoreTest() *Test {
	return &Test{
		AddFunc: func(s *api.AttachedTerminal) error {
			if s == nil {
				return errors.New("nil terminal")
			}
			return nil
		},
	}
}

func (t *Test) Add(s *api.AttachedTerminal) error {
	t.LastAdded = s
	if t.AddFunc != nil {
		return t.AddFunc(s)
	}
	return errdefs.ErrFuncNotSet
}
