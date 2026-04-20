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

//go:build unix

package spawn

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

func TestNewTerminal_Validation(t *testing.T) {
	t.Parallel()
	validSpec := &api.TerminalSpec{SocketFile: "/tmp/x.sock"}

	tests := []struct {
		name string
		spec *api.TerminalSpec
		opts TerminalOptions
		want error
	}{
		{
			name: "missing binary",
			spec: validSpec,
			opts: TerminalOptions{},
			want: ErrBinaryPathRequired,
		},
		{
			name: "nil spec",
			spec: nil,
			opts: TerminalOptions{BinaryPath: "/bin/true"},
			want: ErrSpecRequired,
		},
		{
			name: "missing socket",
			spec: &api.TerminalSpec{},
			opts: TerminalOptions{BinaryPath: "/bin/true"},
			want: ErrSocketPathRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTerminal(context.Background(), tc.spec, tc.opts)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NewTerminal err = %v; want %v", err, tc.want)
			}
		})
	}
}

func TestNewTerminal_CanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	spec := &api.TerminalSpec{SocketFile: "/tmp/spawn-test.sock"}
	_, err := NewTerminal(ctx, spec, TerminalOptions{BinaryPath: "/bin/true"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NewTerminal err = %v; want context.Canceled", err)
	}
}

// TestTerminalHandle_WaitReady_ProcessExits covers the path where the
// spawned child dies before it opens its control socket. WaitReady
// must surface ErrProcessExited rather than hang on the poll loop.
func TestTerminalHandle_WaitReady_ProcessExits(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	spec := &api.TerminalSpec{
		SocketFile: filepath.Join(tmp, "socket"),
	}

	// /bin/true exits immediately — no terminal will ever come up.
	h, err := NewTerminal(context.Background(), spec, TerminalOptions{
		BinaryPath:        "/bin/true",
		ReadyPollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTerminal: %v", err)
	}
	defer func() { _ = h.WaitClose(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = h.WaitReady(ctx)
	if !errors.Is(err, ErrProcessExited) {
		t.Fatalf("WaitReady err = %v; want ErrProcessExited", err)
	}
}

// TestTerminalHandle_PIDAndSocketPath verifies the trivial accessors
// return what NewTerminal recorded, even after the child exits.
func TestTerminalHandle_PIDAndSocketPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	sockPath := filepath.Join(tmp, "socket")
	spec := &api.TerminalSpec{SocketFile: sockPath}

	h, err := NewTerminal(context.Background(), spec, TerminalOptions{
		BinaryPath: "/bin/true",
	})
	if err != nil {
		t.Fatalf("NewTerminal: %v", err)
	}
	if pid := h.PID(); pid <= 0 {
		t.Fatalf("PID = %d; want > 0", pid)
	}
	if got := h.SocketPath(); got != sockPath {
		t.Fatalf("SocketPath = %q; want %q", got, sockPath)
	}
	if got := h.Spec(); got != spec {
		t.Fatalf("Spec = %p; want %p", got, spec)
	}
	// Clean up the reaper goroutine.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.WaitClose(ctx)
}
