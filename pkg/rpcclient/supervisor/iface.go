package supervisor

import (
	"context"
)

type Client interface {
	Detach(ctx context.Context) error
	// add more RPCs as you expose them:
	// List(ctx context.Context) ([]api.Session, error)
	// Attach(ctx context.Context, id api.ID) error
	Close() error
}
