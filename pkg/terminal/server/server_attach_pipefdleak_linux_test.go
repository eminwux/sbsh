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

// TestServer_AttachDetachCycles_NoPipeFDLeak locks in the fix for
// issue #215: setupPipes allocates an os.Pipe pair per Attach that
// cleanupClient (and the Detach fast-path) used to leave open for the
// lifetime of the runner process. The sibling SCM_RIGHTS test counts
// only socket-typed fds because, before this fix, the pipe leak (two
// fds per cycle) would dwarf the socket signal.
//
// This test counts only pipe-typed fds (`/proc/self/fd` entries whose
// readlink starts with `pipe:`) so the SCM_RIGHTS regression and this
// one stay independent. Without the cleanup fix every Attach/Detach
// cycle leaks two pipe fds in the runner process; with it the count is
// flat across cycles.
func TestServer_AttachDetachCycles_NoPipeFDLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h := startTestServer(ctx, t)
	defer h.cleanup()

	client := rpcclient.NewUnix(h.socketPath, h.logger)
	defer client.Close()

	// Warm-up cycle: surface one-shot allocations (lazy pipes, goroutines
	// holding read-side fds) before the baseline.
	if err := attachDetachOnce(ctx, client, "warmup"); err != nil {
		t.Fatalf("warmup attach/detach: %v", err)
	}
	// Detach closes the runner-side conn asynchronously after a 100ms
	// grace window; wait past that so the baseline is steady.
	time.Sleep(300 * time.Millisecond)

	baseline, err := countOpenPipeFDs()
	if err != nil {
		t.Fatalf("countOpenPipeFDs (baseline): %v", err)
	}

	const cycles = 20
	for i := range cycles {
		clientID := api.ID(fmt.Sprintf("pipeleak-%d", i))
		if cycleErr := attachDetachOnce(ctx, client, clientID); cycleErr != nil {
			t.Fatalf("cycle %d: %v", i, cycleErr)
		}
	}
	// Same grace as above: let the runner's deferred conn.Close fire on
	// the last cycle before counting.
	time.Sleep(300 * time.Millisecond)

	after, err := countOpenPipeFDs()
	if err != nil {
		t.Fatalf("countOpenPipeFDs (after): %v", err)
	}

	// Without the cleanup fix every cycle leaks two pipe fds (the
	// pipeOutR/pipeOutW pair allocated in setupPipes), so 20 cycles
	// produced +40. The tolerance covers any in-flight Go-runtime pipes
	// (netpoll wake-up, gc) that briefly appear under load. Anything
	// close to 2*cycles means the leak is back.
	delta := after - baseline
	tolerance := cycles / 2
	if delta > tolerance {
		t.Fatalf(
			"open pipe fd count grew by %d over %d attach/detach cycles (tolerance %d, baseline %d, after %d) — cleanupClient/Detach likely leaking the setupPipes pair again",
			delta,
			cycles,
			tolerance,
			baseline,
			after,
		)
	}
}

func countOpenPipeFDs() (int, error) {
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
		if strings.HasPrefix(target, "pipe:") {
			n++
		}
	}
	return n, nil
}
