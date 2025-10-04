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
