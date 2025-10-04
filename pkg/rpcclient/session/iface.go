package session

import (
	"context"
	"net"
	"sbsh/pkg/api"
)

type Client interface {
	Status(ctx context.Context, status *api.SessionStatusMessage) error
	Resize(ctx context.Context, args *api.ResizeArgs) error
	Detach(ctx context.Context, id *api.ID) error
	// add more RPCs as you expose them:
	// List(ctx context.Context) ([]api.Session, error)
	Attach(ctx context.Context, id *api.ID, response any) (net.Conn, error)
	Close() error
}
