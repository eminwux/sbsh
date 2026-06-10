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
	"io"
	"os"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

// TestSetupShell_StatePollRace reproduces issue #388 Pair B: an attaching
// client polls the State RPC (which copies the whole sr.metadata struct under
// metadataMu.RLock via Metadata()) while the controller goroutine runs
// SetupShell, which read-modify-writes sr.metadata.Spec.Prompt. With the
// pre-fix code the write is lock-free, so a -race build flags the unlocked
// write against the RLock'd reader; the fix routes the write through
// metadataMu. Prompt is reset to "" under the lock between iterations so the
// resolve-default write fires every pass, widening the detection window.
func TestSetupShell_StatePollRace(t *testing.T) {
	sr := newWiredExec(t)
	sr.metadata.Spec.SetPrompt = true

	// Wire a drained pipe as the PTY so SetupShell's Write() neither nil-panics
	// nor blocks on a full pipe buffer.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	go func() { _, _ = io.Copy(io.Discard, pr) }()
	sr.ptmx = pw

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = sr.Metadata()
			}
		}
	}()

	const iterations = 50
	for i := 0; i < iterations; i++ {
		sr.metadataMu.Lock()
		sr.metadata.Spec.Prompt = ""
		sr.metadataMu.Unlock()
		if err := sr.SetupShell(); err != nil {
			t.Fatalf("SetupShell: %v", err)
		}
	}

	close(stop)
	wg.Wait()
}

// TestPostAttachShell_OverlappingAttachesRace reproduces issue #388 Pair A: two
// clients attach concurrently, so two PostAttachShell calls run on separate
// goroutines. One advances Status.State through updateTerminalState (a locked
// write) while the other spin-reads Status.State at the readiness loop. With
// the pre-fix code that spin-read was lock-free, so a -race build flags it
// against the locked write; the fix reads via the state() accessor. The
// terminal is seeded Ready (as OnInitShell leaves it before attaches arrive),
// and each PostAttachShell cycles it back through PostAttach -> Ready, so the
// two goroutines drive interleaved unlocked reads and locked writes.
func TestPostAttachShell_OverlappingAttachesRace(t *testing.T) {
	sr := newWiredExec(t)
	sr.metadata.Status.State = api.Ready

	const (
		attachers  = 2
		iterations = 200
	)
	var wg sync.WaitGroup
	for g := 0; g < attachers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := sr.PostAttachShell(); err != nil {
					t.Errorf("PostAttachShell: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
