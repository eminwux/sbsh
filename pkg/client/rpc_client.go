package client

import (
	"context"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"sbsh/pkg/api"
)

type ClientControllerRPC struct {
	c    *rpc.Client
	conn net.Conn // so we can close on ctx cancel
}

func NewController(sock string) (*ClientControllerRPC, error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	return &ClientControllerRPC{
		c:    rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn)),
		conn: conn,
	}, nil
}

// Run usually a no-op client-side; the daemon runs its own loop.
func (cc *ClientControllerRPC) Run(ctx context.Context) error { return nil }

func (cc *ClientControllerRPC) WaitReady(ctx context.Context) error {
	var reply api.Empty
	return cc.c.Call("Controller.WaitReady", &api.Empty{}, &reply)
}

func (cc *ClientControllerRPC) SetCurrentSession(id api.ID) error {
	var reply api.Empty
	return cc.c.Call("Controller.SetCurrentSession", &api.SessionIDArg{ID: id}, &reply)
}

func (cc *ClientControllerRPC) StartSession(id api.ID) error {
	var reply api.Empty
	return cc.c.Call("Controller.StartSession", &api.SessionIDArg{ID: id}, &reply)
}

// Optional: add Close() to cleanly tear down the client
func (cc *ClientControllerRPC) Close() error {
	_ = cc.c.Close()
	return cc.conn.Close()
}

// TODO
// list all existing sessions
// show session
// stop session
// restart session
// create new session
// attach to a different session
// dettach from current session
