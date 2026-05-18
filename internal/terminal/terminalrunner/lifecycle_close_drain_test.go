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

package terminalrunner

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// TestExecClose_TearsDownAttacherOutWriters is the regression for #230.
//
// Pre-fix, Close() only called c.conn.Close() per client and left each
// attacher's subscriberWriter registered in multiOutW with its drain
// goroutine parked on cond.Wait. The drain only exited indirectly, via the
// dualcopier reader-side onClose firing detach → cleanupClient → aw.Close.
// In this test there is no dualcopier wired up, so the indirect path is
// inert — pre-fix, the N drain goroutines would never exit and the
// post-Close goroutine count would not return to baseline.
//
// Post-fix, Close calls multiOutW.Remove + aw.Close inline per client,
// waking each drain so it can flush, run its onDetach (a no-op here), and
// exit via the defer that closes the conn.
func TestExecClose_TearsDownAttacherOutWriters(t *testing.T) {
	const attacherCount = 5

	sr := newCloseRaceExec(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Real multiOutW so the inline Remove + Close pairing has something to
	// remove from. newCloseRaceExec leaves ptyPipes empty.
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.multiOutW = NewDynamicMultiWriter(logger)
	sr.ptyPipesMu.Unlock()

	baseline := runtime.NumGoroutine()

	for i := range attacherCount {
		id := api.ID(fmt.Sprintf("client-%d", i))
		// net.Pipe gives us two connected in-memory conns. We retain only
		// the runner-side half; the peer is GC'd, which is fine because
		// the drain goroutine's defer closes its end.
		c, _ := net.Pipe()
		aw := newSubscriberWriter(c, defaultSubscriberBufferBytes, nil, logger)
		sr.ptyPipes.multiOutW.Add(aw)
		sr.clients[id] = &ioClient{id: &id, conn: c, outWriter: aw}
		go aw.Run()
	}

	// Let each drain goroutine reach cond.Wait before Close — otherwise a
	// fast Close could win the race and the test would not be probing the
	// "drain is parked and must be woken" failure mode the fix targets.
	time.Sleep(20 * time.Millisecond)

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Poll for goroutine count to return to baseline. The fix is a wakeup
	// not a join, so we allow a bounded grace period for the drains to
	// observe Close and exit. The slack of +1 absorbs unrelated goroutine
	// fluctuations from the runtime/test harness.
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := runtime.NumGoroutine()
		if got <= baseline+1 {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("drain goroutines did not exit within 2s after Close; "+
				"baseline=%d, got=%d (regression of #230)", baseline, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
