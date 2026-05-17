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

//go:build linux

package server_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
)

// TestServer_PausedAttacher_DoesNotHeadOfLineBlock pins down the fix for
// issue #214: the interactive Attach path used to write PTY output to
// client.conn via dualcopier.RunCopier with no SetWriteDeadline and no
// bounded ring. A single attacher whose receiver was paused (suspended
// SSH session, blocked terminal emulator, paused tmux pane) would fill
// the socketpair send buffer and head-of-line block the fan-out — every
// other attacher and the capture sink stopped receiving PTY output until
// the paused peer drained or the runner died.
//
// The fix swaps the writer-side dualcopier for a subscriberWriter-style
// wrapper around client.conn: bounded in-memory ring, per-write
// SetWriteDeadline, and lagged-detach funnelled through cleanupClient.
// This test verifies the end-to-end behavior: with one paused attacher
// and one active drainer, the drainer keeps receiving output, and the
// paused attacher is reaped from metadata.Status.Attachers within a
// bounded wait.
func TestServer_PausedAttacher_DoesNotHeadOfLineBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h := startTestServer(ctx, t)
	defer h.cleanup()

	client := rpcclient.NewUnix(h.socketPath, h.logger)
	defer client.Close()

	// Paused attacher: never read. Simulates a receiver that has stopped
	// draining the PTY output stream (suspended SSH session, blocked
	// emulator, paused tmux pane). The kernel socketpair buffer fills,
	// the server-side drain goroutine's per-write deadline trips, and
	// the bounded ring overflows — all of which must funnel through
	// cleanupClient. attachClient registers t.Cleanup for the conn so
	// the returned value isn't needed in the test body.
	pausedID := api.ID("paused-attacher")
	_ = attachClient(ctx, t, client, pausedID)

	// Drainer: reads continuously. With the head-of-line blocking bug,
	// this attacher would also stall as soon as the paused peer's send
	// buffer filled. With the fix, it keeps receiving PTY output even
	// while the paused peer is being reaped.
	drainID := api.ID("drainer-attacher")
	drainConn := attachClient(ctx, t, client, drainID)

	drainedBytes := startDrainCounter(drainConn)

	// Push ~2 MiB of PTY output. With the default ring at 1 MiB and the
	// Linux socketpair kernel buffer at ~208 KiB, this is comfortably
	// over the threshold where a stalled attacher would have wedged the
	// pre-#214 fan-out. `yes X | head -c …` is deterministic, the shell
	// returns to the prompt afterward, and the SBSH_DONE_MARKER tail
	// makes the run easy to debug from the capture file.
	cmd := []byte("yes X | head -c 2000000; echo SBSH_DONE_MARKER\n")
	if werr := client.Write(ctx, &api.WriteRequest{Data: cmd}); werr != nil {
		t.Fatalf("Write(cmd): %v", werr)
	}

	// Drainer must see at least ~1.5 MiB before the deadline. If the
	// fan-out head-of-line blocks on the paused attacher, drainedBytes
	// stalls well below this threshold.
	waitForDrainerOrFail(t, drainedBytes, 1_500_000, 30*time.Second)

	// The paused attacher must be reaped from metadata.Status.Attachers
	// within a bounded wait. The bound is subscriberWriteTimeout (5s
	// default) per in-flight conn.Write, plus a small margin for the
	// cleanup goroutine to run and rewrite metadata.
	md := waitForAttacherReaped(ctx, t, client, pausedID, 15*time.Second)

	if containsAttacher(md.Status.Attachers, string(pausedID)) {
		t.Fatalf(
			"paused attacher %q still listed in metadata.Status.Attachers after bounded wait: %v",
			pausedID, md.Status.Attachers,
		)
	}
	if !containsAttacher(md.Status.Attachers, string(drainID)) {
		t.Fatalf(
			"drainer attacher %q missing from metadata.Status.Attachers (should still be present): %v",
			drainID, md.Status.Attachers,
		)
	}
}

func attachClient(ctx context.Context, t *testing.T, client rpcclient.Client, id api.ID) net.Conn {
	t.Helper()
	conn, err := client.Attach(ctx, &id, nil)
	if err != nil {
		t.Fatalf("Attach(%s): %v", id, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func startDrainCounter(conn net.Conn) *atomic.Int64 {
	var drained atomic.Int64
	go func() {
		buf := make([]byte, 32*1024)
		for {
			if rderr := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); rderr != nil {
				return
			}
			n, rerr := conn.Read(buf)
			if n > 0 {
				drained.Add(int64(n))
			}
			if rerr != nil {
				return
			}
		}
	}()
	return &drained
}

func waitForDrainerOrFail(t *testing.T, drained *atomic.Int64, want int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for drained.Load() < want && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := drained.Load(); got < want {
		t.Fatalf(
			"drainer only received %d bytes (want >= %d) before timeout — fan-out likely head-of-line blocked on the paused attacher",
			got,
			want,
		)
	}
}

func waitForAttacherReaped(
	ctx context.Context,
	t *testing.T,
	client rpcclient.Client,
	id api.ID,
	timeout time.Duration,
) api.TerminalDoc {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var md api.TerminalDoc
	for time.Now().Before(deadline) {
		if mdErr := client.Metadata(ctx, &md); mdErr != nil {
			t.Fatalf("Metadata: %v", mdErr)
		}
		if !containsAttacher(md.Status.Attachers, string(id)) {
			return md
		}
		time.Sleep(100 * time.Millisecond)
	}
	return md
}

func containsAttacher(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
