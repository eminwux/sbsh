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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
)

// TestServer_AttachDetachCycles_NoFDLeak locks in the fix for issue #212:
// the JSON-RPC codec used to leak the sender-side end of the SCM_RIGHTS
// socketpair on every Attach (and Subscribe). Each leaked fd kept the
// peer's send buffer alive in the terminal process, eventually wedging
// the PTY fanout once the buffer filled (~208 KiB).
//
// The test counts only socket-typed fds (`/proc/self/fd` entries whose
// readlink starts with `socket:`) — the runner has unrelated pipe churn
// per Attach that would otherwise dwarf the signal. Without the codec
// fix every Attach leaks one socketpair fd; with it the count is flat
// across cycles.
func TestServer_AttachDetachCycles_NoFDLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h := startTestServer(ctx, t)
	defer h.cleanup()

	client := rpcclient.NewUnix(h.socketPath, h.logger)
	defer client.Close()

	// One warm-up cycle to surface any one-shot allocations (lazy
	// pipes, goroutines holding read-side fds) before the baseline.
	if err := attachDetachOnce(ctx, client, "warmup"); err != nil {
		t.Fatalf("warmup attach/detach: %v", err)
	}
	// Detach closes the runner-side conn asynchronously after a 100ms
	// grace window. Wait past that so the baseline is steady.
	time.Sleep(300 * time.Millisecond)

	baseline, err := countOpenSocketFDs()
	if err != nil {
		t.Fatalf("countOpenSocketFDs (baseline): %v", err)
	}

	const cycles = 20
	for i := range cycles {
		clientID := api.ID(fmt.Sprintf("leakcheck-%d", i))
		if cycleErr := attachDetachOnce(ctx, client, clientID); cycleErr != nil {
			t.Fatalf("cycle %d: %v", i, cycleErr)
		}
	}
	// Same grace as above: let the runner's deferred conn.Close fire
	// on the last cycle before counting.
	time.Sleep(300 * time.Millisecond)

	after, err := countOpenSocketFDs()
	if err != nil {
		t.Fatalf("countOpenSocketFDs (after): %v", err)
	}

	// Without the codec fix every cycle leaks one server-side socket
	// fd, so 20 cycles produced +20. The small tolerance covers any
	// in-flight rpcclient dialed conns that haven't been GC'd yet.
	// Anything close to `cycles` means the leak is back.
	delta := after - baseline
	tolerance := cycles / 4
	if delta > tolerance {
		t.Fatalf(
			"open socket fd count grew by %d over %d attach/detach cycles (tolerance %d, baseline %d, after %d) — codec.WriteResponse likely leaking SCM_RIGHTS fds again",
			delta,
			cycles,
			tolerance,
			baseline,
			after,
		)
	}

	var md api.TerminalDoc
	if mdErr := client.Metadata(ctx, &md); mdErr != nil {
		t.Fatalf("Metadata: %v", mdErr)
	}
	if got := len(md.Status.Attachers); got != 0 {
		t.Fatalf("metadata.Status.Attachers has %d ghost entries after detach cycles, want 0", got)
	}
}

func attachDetachOnce(ctx context.Context, client rpcclient.Client, id api.ID) error {
	conn, err := client.Attach(ctx, &id, nil)
	if err != nil {
		return fmt.Errorf("Attach(%s): %w", id, err)
	}
	if detachErr := client.Detach(ctx, &id); detachErr != nil {
		_ = conn.Close()
		return fmt.Errorf("Detach(%s): %w", id, detachErr)
	}
	if closeErr := conn.Close(); closeErr != nil {
		return fmt.Errorf("conn.Close(%s): %w", id, closeErr)
	}
	return nil
}

func countOpenSocketFDs() (int, error) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		target, lerr := os.Readlink("/proc/self/fd/" + e.Name())
		if lerr != nil {
			// fd may have been reaped between ReadDir and Readlink; skip.
			continue
		}
		if strings.HasPrefix(target, "socket:") {
			n++
		}
	}
	return n, nil
}
