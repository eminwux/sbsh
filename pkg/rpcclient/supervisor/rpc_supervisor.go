package supervisor

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

func (c *client) Close() error { return nil } // no long-lived conn

func (c *client) call(ctx context.Context, method string, in, out any) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	rpcc := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))

	errCh := make(chan error, 1)
	go func() { errCh <- rpcc.Call(method, in, out) }()

	select {
	case <-ctx.Done():
		// Nudge the connection so the blocked call returns quickly.
		_ = conn.SetDeadline(time.Now().Add(10 * time.Millisecond))
		_ = conn.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// dettach from current session
func (c *client) Detach(ctx context.Context) error {
	return c.call(ctx, "SupervisorController.Detach", &api.Empty{}, &api.Empty{})
}

// TODO
// list all existing sessions - X no need to do it via sup
// show session X - no need to do it via sup
// stop session Y - goes to session
// restart session - goes to session
// create new session -
// attach to a different session
