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
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

type Dialer func(ctx context.Context) (net.Conn, error)

type client struct {
	dial Dialer
}

func (c *client) Close() error { return nil } // no long-lived conn

func (c *client) call(ctx context.Context, method string, in, out any) error {
	conn, errDial := c.dial(ctx)
	if errDial != nil {
		return errDial
	}
	defer conn.Close()

	rpcc := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))

	errCh := make(chan error, 1)
	go func() {
		errCh <- rpcc.Call(method, in, out)
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		// Nudge the connection so the blocked call returns quickly.
		//nolint:mnd // short deadline to unblock
		_ = conn.SetDeadline(time.Now().Add(10 * time.Millisecond))
		_ = conn.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *client) Detach(ctx context.Context) error {
	return c.call(ctx, api.ClientMethodDetach, &api.Empty{}, &api.Empty{})
}

func (c *client) Ping(ctx context.Context, ping *api.PingMessage, pong *api.PingMessage) error {
	if ping == nil {
		ping = &api.PingMessage{}
	}
	if pong == nil {
		return c.call(ctx, api.ClientMethodPing, ping, &api.PingMessage{})
	}
	return c.call(ctx, api.ClientMethodPing, ping, pong)
}

func (c *client) Metadata(ctx context.Context, doc *api.ClientDoc) error {
	if doc == nil {
		return c.call(ctx, api.ClientMethodMetadata, &api.Empty{}, &api.ClientDoc{})
	}
	return c.call(ctx, api.ClientMethodMetadata, &api.Empty{}, doc)
}

func (c *client) State(ctx context.Context, state *api.ClientStatusMode) error {
	if state == nil {
		var dummy api.ClientStatusMode
		return c.call(ctx, api.ClientMethodState, &api.Empty{}, &dummy)
	}
	return c.call(ctx, api.ClientMethodState, &api.Empty{}, state)
}

func (c *client) Stop(ctx context.Context, args *api.StopArgs) error {
	if args == nil {
		args = &api.StopArgs{}
	}
	return c.call(ctx, api.ClientMethodStop, args, &api.Empty{})
}
