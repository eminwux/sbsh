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

package clientrpc

import "github.com/eminwux/sbsh/pkg/api"

type ClientControllerRPC struct {
	Core api.ClientController // the real server-side controller
}

// WaitReady Optional: usually you don’t expose Run over RPC because it blocks.
func (s *ClientControllerRPC) WaitReady(_ *api.Empty, _ *api.Empty) error {
	return s.Core.WaitReady()
}

func (s *ClientControllerRPC) Detach(_ *api.Empty, _ *api.Empty) error {
	return s.Core.Detach()
}

func (s *ClientControllerRPC) Ping(in *api.PingMessage, out *api.PingMessage) error {
	pong, err := s.Core.Ping(in)
	if err != nil {
		return err
	}
	*out = *pong
	return nil
}

func (s *ClientControllerRPC) Metadata(_ api.Empty, doc *api.ClientDoc) error {
	md, err := s.Core.Metadata()
	if err != nil {
		return err
	}
	*doc = *md
	return nil
}

func (s *ClientControllerRPC) State(_ api.Empty, state *api.ClientStatusMode) error {
	st, err := s.Core.State()
	if err != nil {
		return err
	}
	*state = *st
	return nil
}

func (s *ClientControllerRPC) Stop(args *api.StopArgs, _ *api.Empty) error {
	return s.Core.Stop(args)
}
