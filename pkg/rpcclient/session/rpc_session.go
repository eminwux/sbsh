package session

import (
	"context"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"sbsh/pkg/api"
	"time"
)

type Dialer func(ctx context.Context) (net.Conn, error)

type client struct {
	dial Dialer
}

func (c *client) call(ctx context.Context, method string, in, out any) error {
	// retry delays (3 attempts: immediate, 200ms later, 400ms later)
	delays := []time.Duration{0, 200 * time.Millisecond, 400 * time.Millisecond}
	var lastErr error

	for _, d := range delays {
		if d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				// wait before retry
			}
		}

		conn, err := c.dial(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		rpcc := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))
		errCh := make(chan error, 1)
		go func() { errCh <- rpcc.Call(method, in, out) }()

		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now().Add(10 * time.Millisecond))
			_ = conn.Close()
			return ctx.Err()
		case err = <-errCh:
			_ = conn.Close()
			if err == nil {
				return nil // success
			}
			lastErr = err
		}
	}

	return lastErr
}

func (c *client) Close() error { return nil } // no long-lived conn

func (c *client) Status(ctx context.Context, status *api.SessionStatus) error {
	return c.call(ctx, api.SessionMethodStatus, &api.Empty{}, status)
}

// dettach from current session
func (c *client) Resize(ctx context.Context, args *api.ResizeArgs) error {
	return c.call(ctx, api.SessionMethodResize, args, &api.Empty{})
}

// dettach from current session
func (c *client) Detach(ctx context.Context) error {
	return c.call(ctx, api.SessionMethodDetach, &api.Empty{}, &api.Empty{})
}

// TODO
// list all existing sessions - X no need to do it via sup
// show session X - no need to do it via sup
// stop session Y - goes to session
// restart session - goes to session
// create new session -
// attach to a different session
