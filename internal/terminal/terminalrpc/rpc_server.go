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

package terminalrpc

import "github.com/eminwux/sbsh/pkg/api"

type TerminalControllerRPC struct {
	Core api.TerminalController
}

func (s *TerminalControllerRPC) Ping(in *api.PingMessage, out *api.PingMessage) error {
	pong, err := s.Core.Ping(in)
	if err != nil {
		return err
	}
	*out = *pong
	return nil
}

func (s *TerminalControllerRPC) Resize(args api.ResizeArgs, _ *api.Empty) error {
	s.Core.Resize(args)
	return nil
}

func (s *TerminalControllerRPC) Detach(id *api.ID, _ *api.Empty) error {
	return s.Core.Detach(id)
}

func (s *TerminalControllerRPC) Attach(id *api.ID, response *api.ResponseWithFD) error {
	return s.Core.Attach(id, response)
}

func (s *TerminalControllerRPC) Metadata(_ api.Empty, metadata *api.TerminalMetadata) error {
	md, err := s.Core.Metadata()
	if err != nil {
		return err
	}
	*metadata = *md
	return nil
}
