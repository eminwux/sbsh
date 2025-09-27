package supervisor

import (
	"context"
	"net"
	"time"
)

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
