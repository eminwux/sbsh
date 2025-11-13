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
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"time"

	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

type Dialer func(ctx context.Context) (net.Conn, error)

type client struct {
	dial   Dialer
	logger *slog.Logger
}

type (
	Option   func(*unixOpts)
	unixOpts struct {
		DialTimeout time.Duration
	}
)

func WithDialTimeout(d time.Duration) Option {
	return func(o *unixOpts) { o.DialTimeout = d }
}

// NewUnix returns a ctx-aware client that dials a Unix socket per call.
func NewUnix(sockPath string, logger *slog.Logger, opts ...Option) Client {
	//nolint:mnd // default timeout
	cfg := unixOpts{DialTimeout: 5 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	dialer := func(ctx context.Context) (net.Conn, error) {
		d := net.Dialer{Timeout: cfg.DialTimeout}
		return d.DialContext(ctx, "unix", sockPath)
	}
	return &client{dial: dialer, logger: logger}
}

// --- internal: generic call path with pluggable codec (handles retries/timeouts) ---.
func (c *client) callWithCodec(
	ctx context.Context,
	method string,
	in, out any,
	newCodec func(conn net.Conn) (rpc.ClientCodec, func(), error), // returns codec + cleanup
) error {
	delays := []time.Duration{0, 200 * time.Millisecond, 400 * time.Millisecond}
	var lastErr error

	for attempt, d := range delays {
		if d > 0 {
			c.logger.DebugContext(ctx, "delaying before retry", "attempt", attempt, "delay", d)
			select {
			case <-ctx.Done():
				c.logger.WarnContext(ctx, "context done before retry", "attempt", attempt, "error", ctx.Err())
				return ctx.Err()
			case <-time.After(d):
			}
		}

		c.logger.DebugContext(ctx, "dialing connection", "attempt", attempt)
		conn, err := c.dial(ctx)
		if err != nil {
			c.logger.WarnContext(ctx, "dial failed", "attempt", attempt, "error", err)
			lastErr = err
			continue
		}

		c.logger.DebugContext(ctx, "creating codec", "attempt", attempt)
		codec, cleanup, err := newCodec(conn)
		if err != nil {
			c.logger.ErrorContext(ctx, "codec creation failed", "attempt", attempt, "error", err)
			_ = conn.Close()
			lastErr = err
			continue
		}

		c.logger.DebugContext(ctx, "starting RPC call", "method", method, "attempt", attempt)
		rpcc := rpc.NewClientWithCodec(codec)
		errCh := make(chan error, 1)
		go func() {
			errCh <- rpcc.Call(method, in, out)
			close(errCh)
		}()

		select {
		case <-ctx.Done():
			c.logger.WarnContext(
				ctx,
				"context done during RPC call",
				"method",
				method,
				"attempt",
				attempt,
				"error",
				ctx.Err(),
			)
			_ = conn.SetDeadline(time.Now().Add(10 * time.Millisecond))
			_ = rpcc.Close()
			if cleanup != nil {
				cleanup()
			}
			return ctx.Err()
		case err = <-errCh:
			_ = rpcc.Close()
			if cleanup != nil {
				cleanup()
			}
			if err == nil {
				c.logger.InfoContext(ctx, "RPC call succeeded", "method", method, "attempt", attempt)
				return nil
			}
			c.logger.ErrorContext(ctx, "RPC call failed", "method", method, "attempt", attempt, "error", err)
			lastErr = err
		}
	}
	c.logger.ErrorContext(ctx, "all attempts failed for RPC call", "method", method, "error", lastErr)
	return lastErr
}

// --- convenience: plain JSON-RPC (no FD passing) ---.
func (c *client) call(ctx context.Context, logger *slog.Logger, method string, in, out any) error {
	logger.DebugContext(ctx, "preparing codec for call", "method", method)
	return c.callWithCodec(ctx, method, in, out, func(conn net.Conn) (rpc.ClientCodec, func(), error) {
		// If this is a Unix domain socket, use the Unix-aware logger.
		if u, ok := conn.(*net.UnixConn); ok {
			logger.DebugContext(ctx, "using LoggingConnUnix for codec", "method", method)
			luc := &shared.LoggingConnUnix{
				UnixConn:    u,
				Logger:      logger,
				PrefixWrite: "client->server",
				PrefixRead:  "server->client",
			}
			return jsonrpc.NewClientCodec(luc), func() { _ = conn.Close() }, nil
		}

		// Otherwise fall back to the generic logger.
		logger.DebugContext(ctx, "using LoggingConn for codec", "method", method)
		lc := &shared.LoggingConn{
			Conn:        conn,
			Logger:      logger,
			PrefixWrite: "client->server",
			PrefixRead:  "server->client",
		}
		return jsonrpc.NewClientCodec(lc), func() { _ = conn.Close() }, nil
	})
}

func (c *client) Close() error { return nil } // stateless client

func (c *client) Ping(ctx context.Context, ping *api.PingMessage, pong *api.PingMessage) error {
	return c.call(ctx, c.logger, api.TerminalMethodPing, ping, pong)
}

func (c *client) Resize(ctx context.Context, args *api.ResizeArgs) error {
	return c.call(ctx, c.logger, api.TerminalMethodResize, args, &api.Empty{})
}

func (c *client) Detach(ctx context.Context, id *api.ID) error {
	return c.call(ctx, c.logger, api.TerminalMethodDetach, id, &api.Empty{})
}

// Attach (uses FD-aware codec) returns the IO net.Conn built from the FD sent via SCM_RIGHTS; also fills 'out' with JSON result.
func (c *client) Attach(ctx context.Context, id *api.ID, out any) (net.Conn, error) {
	var gotFDs []int

	c.logger.InfoContext(ctx, "Attach RPC call starting")

	err := c.callWithCodec(
		ctx,
		api.TerminalMethodAttach,
		id,
		out,
		func(conn net.Conn) (rpc.ClientCodec, func(), error) {
			c.logger.DebugContext(ctx, "preparing codec for Attach")
			uconn, ok := conn.(*net.UnixConn)
			if !ok {
				c.logger.ErrorContext(ctx, "attach: not a *net.UnixConn")
				_ = conn.Close()
				return nil, nil, errors.New("attach: not a *net.UnixConn")
			}
			codec := newUnixJSONClientCodec(uconn, c.logger)
			c.logger.DebugContext(ctx, "Attach RPC call completed")

			cleanup := func() {
				gotFDs = codec.takeLastFDs()
				_ = conn.Close()
				c.logger.DebugContext(ctx, "cleanup finished, FDs taken", "fd_count", len(gotFDs))
			}

			return codec, cleanup, nil
		},
	)

	c.logger.DebugContext(ctx, "callWithCodec finished for Attach")
	if err != nil {
		c.logger.ErrorContext(ctx, "Attach failed", "error", err)
		return nil, err
	}

	if len(gotFDs) == 0 {
		c.logger.ErrorContext(ctx, "attach: server did not send IO fd")
		return nil, errors.New("attach: server did not send IO fd")
	}

	c.logger.DebugContext(ctx, "converting FD to net.Conn", "fd", gotFDs[0])

	f := os.NewFile(uintptr(gotFDs[0]), "io")
	defer f.Close()
	ioConn, err := net.FileConn(f)
	if err != nil {
		c.logger.ErrorContext(ctx, "FileConn failed", "error", err)
		return nil, fmt.Errorf("attach: FileConn: %w", err)
	}
	c.logger.InfoContext(ctx, "Attach succeeded, returning net.Conn")
	return ioConn, nil
}

func (c *client) Metadata(ctx context.Context, metadata *api.TerminalDoc) error {
	err := c.call(ctx, c.logger, api.TerminalMethodMetadata, &api.Empty{}, metadata)
	if err != nil {
		c.logger.ErrorContext(ctx, "Metadata call failed", "error", err)
		return fmt.Errorf("Metadata call failed: %w", err)
	}
	return nil
}

func (c *client) State(ctx context.Context, state *api.TerminalStatusMode) error {
	err := c.call(ctx, c.logger, api.TerminalMethodState, &api.Empty{}, state)
	if err != nil {
		c.logger.ErrorContext(ctx, "State call failed", "error", err)
		return fmt.Errorf("State call failed: %w", err)
	}
	return nil
}
