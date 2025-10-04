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

package supervisorrpc

import (
	"sbsh/pkg/api"
)

type SupervisorControllerRPC struct {
	Core api.SupervisorController // the real server-side controller
}

// Optional: usually you don’t expose Run over RPC because it blocks.
func (s *SupervisorControllerRPC) WaitReady(_ *api.Empty, _ *api.Empty) error {
	return s.Core.WaitReady()
}

func (s *SupervisorControllerRPC) Detach(_ *api.Empty, _ *api.Empty) error {
	return s.Core.Detach()
}

// TODO
// show current attach session
// attach to a different session
// detach from session and exit
