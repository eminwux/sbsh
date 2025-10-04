package session

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"time"
)

type Dialer func(ctx context.Context) (net.Conn, error)

type client struct {
	dial Dialer
}

type Option func(*unixOpts)
type unixOpts struct {
	DialTimeout time.Duration
}

func WithDialTimeout(d time.Duration) Option {
	return func(o *unixOpts) { o.DialTimeout = d }
}

// NewUnix returns a ctx-aware client that dials a Unix socket per call.
func NewUnix(sockPath string, opts ...Option) Client {
	cfg := unixOpts{DialTimeout: 5 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	dialer := func(ctx context.Context) (net.Conn, error) {
		d := net.Dialer{Timeout: cfg.DialTimeout}
		return d.DialContext(ctx, "unix", sockPath)
	}
	return &client{dial: dialer}
}

// --- internal: generic call path with pluggable codec (handles retries/timeouts) ---
func (c *client) callWithCodec(
	ctx context.Context,
	method string,
	in, out any,
	newCodec func(conn net.Conn) (rpc.ClientCodec, func(), error), // returns codec + cleanup
) error {
	// retry delays (3 attempts: immediate, 200ms, 400ms)
	delays := []time.Duration{0, 200 * time.Millisecond, 400 * time.Millisecond}
	var lastErr error

	for _, d := range delays {
		if d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}

		conn, err := c.dial(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		codec, cleanup, err := newCodec(conn)
		if err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}

		rpcc := rpc.NewClientWithCodec(codec)
		errCh := make(chan error, 1)
		go func() { errCh <- rpcc.Call(method, in, out) }()

		select {
		case <-ctx.Done():
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
				return nil
			}
			lastErr = err
		}
	}
	return lastErr
}

// --- convenience: plain JSON-RPC (no FD passing) ---
func (c *client) call(ctx context.Context, method string, in, out any) error {
	return c.callWithCodec(ctx, method, in, out, func(conn net.Conn) (rpc.ClientCodec, func(), error) {
		// If this is a Unix domain socket, use the Unix-aware logger.
		if u, ok := conn.(*net.UnixConn); ok {
			luc := &common.LoggingConnUnix{
				UnixConn:    u,
				PrefixWrite: "client->server",
				PrefixRead:  "server->client",
			}
			return jsonrpc.NewClientCodec(luc), func() { _ = conn.Close() }, nil
		}

		// Otherwise fall back to the generic logger.
		lc := &common.LoggingConn{
			Conn:        conn,
			PrefixWrite: "client->server",
			PrefixRead:  "server->client",
		}
		return jsonrpc.NewClientCodec(lc), func() { _ = conn.Close() }, nil
	})
}

func (c *client) Close() error { return nil } // stateless client

// --- Public methods (no FD passing) ---
func (c *client) Status(ctx context.Context, status *api.SessionStatusMessage) error {
	return c.call(ctx, api.SessionMethodStatus, &api.Empty{}, status)
}

func (c *client) Resize(ctx context.Context, args *api.ResizeArgs) error {
	return c.call(ctx, api.SessionMethodResize, args, &api.Empty{})
}

func (c *client) Detach(ctx context.Context, id *api.ID) error {
	return c.call(ctx, api.SessionMethodDetach, id, &api.Empty{})
}

// --- Attach (uses FD-aware codec) ---
// Returns the IO net.Conn built from the FD sent via SCM_RIGHTS; also fills 'out' with JSON result.
func (c *client) Attach(ctx context.Context, id *api.ID, out any) (net.Conn, error) {
	var (
		gotFDs []int
	)

	slog.Debug("[client] Attach RPC call starting")

	// run through the same retry/timeout path, but with our FD-aware codec
	err := c.callWithCodec(ctx, api.SessionMethodAttach, id, out, func(conn net.Conn) (rpc.ClientCodec, func(), error) {
		// must be *net.UnixConn for ReadMsgUnix/FDs
		uconn, ok := conn.(*net.UnixConn)
		if !ok {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("attach: not a *net.UnixConn")
		}
		codec := newUnixJSONClientCodec(uconn)
		slog.Debug("[client] Attach RPC call completed")

		// after the rpc Call completes, we’ll harvest the FDs from the codec in the cleanup
		cleanup := func() {
			gotFDs = codec.takeLastFDs()
			_ = conn.Close()
		}
		slog.Debug("[client] Cleanup finished")

		return codec, cleanup, nil
	})

	slog.Debug("[client] Call with codec finished")
	if err != nil {
		return nil, err
	}

	if len(gotFDs) == 0 {
		return nil, fmt.Errorf("attach: server did not send IO fd")
	}

	slog.Debug("[client] converting FD to net.Conn")

	// convert FD to net.Conn
	f := os.NewFile(uintptr(gotFDs[0]), "io")
	defer f.Close()
	ioConn, err := net.FileConn(f)
	if err != nil {
		return nil, fmt.Errorf("attach: FileConn: %w", err)
	}
	return ioConn, nil
}
