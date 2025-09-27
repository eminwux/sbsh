package session

import (
	"context"
	"sbsh/pkg/api"
)

type Client interface {
	Status(ctx context.Context, status *api.SessionStatus) error
	Resize(ctx context.Context, args *api.ResizeArgs) error
	Detach(ctx context.Context) error
	// add more RPCs as you expose them:
	// List(ctx context.Context) ([]api.Session, error)
	// Attach(ctx context.Context, id api.ID) error
	Close() error
}
