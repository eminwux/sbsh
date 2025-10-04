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

package sessionrpc

import (
	"sbsh/pkg/api"
)

type SessionControllerRPC struct {
	Core api.SessionController
}

func (r *SessionControllerRPC) Status(_ *api.Empty, out *api.SessionStatusMessage) error {
	*out = api.SessionStatusMessage{Message: r.Core.Status()}
	return nil
}
func (s *SessionControllerRPC) Resize(args api.ResizeArgs, _ *api.Empty) error {
	s.Core.Resize(args)
	return nil
}

func (s *SessionControllerRPC) Detach(id *api.ID, _ *api.Empty) error {
	return s.Core.Detach(id)
}

func (s *SessionControllerRPC) Attach(id *api.ID, response *api.ResponseWithFD) error {
	return s.Core.Attach(id, response)
}

// TODO
// show session details, including attach status
// attach, redirects pipe output/input to socket
// dettach, redirects pipe output to log, input to null
// close session
// restart session
