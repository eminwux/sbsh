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

package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/eminwux/sbsh/pkg/terminal/server"
)

// clientStream collects everything an attached client receives.
type clientStream struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *clientStream) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *clientStream) contains(s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Contains(c.buf.Bytes(), []byte(s))
}

func attachOverlayClient(ctx context.Context, t *testing.T, h *testHarness, req *api.AttachRequest) *clientStream {
	t.Helper()
	client := rpcclient.NewUnix(h.socketPath, h.logger)
	t.Cleanup(func() { _ = client.Close() })
	conn, err := client.Attach(ctx, req, nil)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cs := &clientStream{}
	go func(c net.Conn) { _, _ = io.Copy(cs, c) }(conn)
	return cs
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestServer_Overlays drives the public overlay API against a real PTY and
// real attach sockets: Clients reports the opt-out, a toast and takeover
// frames reach the overlay client only, and End resumes live output.
func TestServer_Overlays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := startTestServer(ctx, t)
	defer h.cleanup()

	withOverlays := attachOverlayClient(ctx, t, h, &api.AttachRequest{ClientID: "with"})
	optedOut := attachOverlayClient(ctx, t, h, &api.AttachRequest{ClientID: "without", NoOverlays: true})

	eventually(t, "both clients listed", func() bool { return len(h.srv.Clients()) == 2 })
	for _, ci := range h.srv.Clients() {
		if want := ci.ID == "without"; ci.NoOverlays != want {
			t.Errorf("client %s NoOverlays = %v, want %v", ci.ID, ci.NoOverlays, want)
		}
	}

	id, err := h.srv.ShowToast(ctx, server.Target{All: true}, server.Toast{Lines: []string{"overlay toast"}})
	if err != nil {
		t.Fatalf("ShowToast: %v", err)
	}
	eventually(t, "toast on the overlay client", func() bool { return withOverlays.contains("overlay toast") })
	if err = h.srv.DismissToast(id); err != nil {
		t.Fatalf("DismissToast: %v", err)
	}

	tk, err := h.srv.BeginTakeover(ctx, server.Target{All: true}, server.TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	if got := tk.Clients(); len(got) != 1 || got[0] != "with" {
		t.Errorf("takeover clients = %v, want [with]", got)
	}
	if err = tk.Write([]byte("TAKEOVER FRAME")); err != nil {
		t.Fatalf("Takeover.Write: %v", err)
	}
	eventually(t, "frame on the overlay client", func() bool { return withOverlays.contains("TAKEOVER FRAME") })
	if err = tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	// Live output still reaches both clients after the takeover.
	rpc := rpcclient.NewUnix(h.socketPath, h.logger)
	defer rpc.Close()
	if err = rpc.Write(ctx, &api.WriteRequest{Data: []byte("echo after-$((40+2))\n")}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	eventually(t, "live output after End", func() bool {
		return withOverlays.contains("after-42") && optedOut.contains("after-42")
	})

	if optedOut.contains("overlay toast") || optedOut.contains("TAKEOVER FRAME") {
		t.Errorf("opted-out client received overlay bytes")
	}
}

func TestServer_OverlaysBeforeServe(t *testing.T) {
	spec := &api.TerminalSpec{
		ID:         "not-started",
		Command:    "/bin/sh",
		RunPath:    t.TempDir(),
		SocketFile: "unused.sock",
	}
	srv, err := server.New(spec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if got := srv.Clients(); got != nil {
		t.Errorf("Clients before Serve = %v, want nil", got)
	}
	if _, err = srv.ShowToast(ctx, server.Target{All: true}, server.Toast{Lines: []string{"x"}}); !errors.Is(
		err,
		server.ErrNotStarted,
	) {
		t.Errorf("ShowToast before Serve: got %v", err)
	}
	if err = srv.DismissToast(1); !errors.Is(err, server.ErrNotStarted) {
		t.Errorf("DismissToast before Serve: got %v", err)
	}
	if _, err = srv.BeginTakeover(ctx, server.Target{All: true}, server.TakeoverOptions{}); !errors.Is(
		err,
		server.ErrNotStarted,
	) {
		t.Errorf("BeginTakeover before Serve: got %v", err)
	}
}
