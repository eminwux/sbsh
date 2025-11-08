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

package terminal

import (
	"context"
	"net"

	"github.com/eminwux/sbsh/pkg/api"
)

type Client interface {
	Ping(ctx context.Context, ping *api.PingMessage, pong *api.PingMessage) error
	Resize(ctx context.Context, args *api.ResizeArgs) error
	Detach(ctx context.Context, id *api.ID) error
	Attach(ctx context.Context, supervisorID *api.ID, response any) (net.Conn, error)
	Close() error
	Metadata(ctx context.Context, metadata *api.TerminalMetadata) error
	State(ctx context.Context, state *api.TerminalStatusMode) error
}
