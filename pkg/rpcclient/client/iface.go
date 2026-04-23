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

package client

import (
	"context"

	"github.com/eminwux/sbsh/pkg/api"
)

type Client interface {
	Detach(ctx context.Context) error
	Ping(ctx context.Context, ping *api.PingMessage, pong *api.PingMessage) error
	Metadata(ctx context.Context, doc *api.ClientDoc) error
	State(ctx context.Context, state *api.ClientStatusMode) error
	Stop(ctx context.Context, args *api.StopArgs) error
	Close() error
}
