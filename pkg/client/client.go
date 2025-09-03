package client

import (
	"context"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"sbsh/pkg/api"
)

type Controller struct {
	c    *rpc.Client
	conn net.Conn // so we can close on ctx cancel
}

func NewController(sock string) (*Controller, error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	return &Controller{
		c:    rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn)),
		conn: conn,
	}, nil
}

// Run usually a no-op client-side; the daemon runs its own loop.
func (cc *Controller) Run(ctx context.Context) error { return nil }

func (cc *Controller) WaitReady(ctx context.Context) error {
	var reply api.Empty
	return cc.c.Call("Controller.WaitReady", &api.Empty{}, &reply)
}

func (cc *Controller) AddSession(spec *api.SessionSpec) {
	_ = cc.c.Call("Controller.AddSession", &api.AddSessionArgs{Spec: *spec}, &api.Empty{})
}

func (cc *Controller) SetCurrentSession(id api.SessionID) error {
	var reply api.Empty
	return cc.c.Call("Controller.SetCurrentSession", &api.SessionIDArg{ID: id}, &reply)
}

func (cc *Controller) StartSession(id api.SessionID) error {
	var reply api.Empty
	return cc.c.Call("Controller.StartSession", &api.SessionIDArg{ID: id}, &reply)
}

// Optional: add Close() to cleanly tear down the client
func (cc *Controller) Close() error {
	_ = cc.c.Close()
	return cc.conn.Close()
}

////////////////////// INTERNAL

// func withDeadline(ctx context.Context, conn net.Conn) func() {
// 	if deadline, ok := ctx.Deadline(); ok {
// 		_ = conn.SetDeadline(deadline)
// 		return func() { _ = conn.SetDeadline(time.Time{}) }
// 	}
// 	// cancel on ctx.Done by closing the conn (brutal but effective)
// 	done := make(chan struct{})
// 	go func() { <-ctx.Done(); _ = conn.Close(); close(done) }()
// 	return func() { <-done } // wait goroutine to finish before reuse
// }
