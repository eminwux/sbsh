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

package sessionrunner

import (
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
)

type SessionRunner interface {
	OpenSocketCtrl() error
	StartServer(sc *sessionrpc.SessionControllerRPC, readyCh chan error)
	StartSession(evCh chan<- SessionRunnerEvent) error
	ID() api.ID
	Close(reason error) error
	Resize(args api.ResizeArgs)
	CreateMetadata() error
	Detach(id *api.ID) error
	Attach(id *api.ID, response *api.ResponseWithFD) error
}
