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

// ExtraHandler is a custom JSON-RPC service registered alongside the
// built-in TerminalController on the same listener. Name is the
// net/rpc service name (the part before the dot in a "Service.Method"
// wire call); Receiver is any value whose exported methods follow
// net/rpc's signature scheme: func(args T1, reply *T2) error. Custom
// services are namespaced by Name, so their methods never collide with
// the built-in TerminalController methods; a Name equal to an
// already-registered service (including the built-in one) is rejected
// at registration by net/rpc.
type ExtraHandler struct {
	Name     string
	Receiver any
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

func (s *TerminalControllerRPC) Metadata(_ api.Empty, metadata *api.TerminalDoc) error {
	md, err := s.Core.Metadata()
	if err != nil {
		return err
	}
	*metadata = *md
	return nil
}

func (s *TerminalControllerRPC) State(_ api.Empty, state *api.TerminalStatusMode) error {
	st, err := s.Core.State()
	if err != nil {
		return err
	}
	*state = *st
	return nil
}

func (s *TerminalControllerRPC) Stop(args *api.StopArgs, _ *api.Empty) error {
	return s.Core.Stop(args)
}

func (s *TerminalControllerRPC) Write(req *api.WriteRequest, _ *api.Empty) error {
	return s.Core.Write(req)
}

func (s *TerminalControllerRPC) Subscribe(req *api.SubscribeRequest, response *api.ResponseWithFD) error {
	return s.Core.Subscribe(req, response)
}
