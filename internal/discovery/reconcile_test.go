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

package discovery

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

func writeTerminalDocFull(t *testing.T, runPath, id string, state api.TerminalStatusMode, pid int, pidStart uint64) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Spec:       api.TerminalSpec{ID: api.ID(id), RunPath: runPath},
		Status: api.TerminalStatus{
			State:           state,
			Pid:             pid,
			PidStart:        pidStart,
			TerminalRunPath: dir,
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); writeErr != nil {
		t.Fatalf("write metadata.json: %v", writeErr)
	}
}

func readTerminalState(t *testing.T, runPath, id string) api.TerminalStatusMode {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(runPath, defaults.TerminalsRunPath, id, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var doc api.TerminalDoc
	if unmarshalErr := json.Unmarshal(b, &doc); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	return doc.Status.State
}

// TestReconcileTerminals_RejectsRecycledPid asserts a live PID with a
// non-matching pidStart token is treated as a recycled PID and the terminal
// is flipped to Exited. Linux-only because the pidStart token is /proc-backed
// and zero on other platforms (where the rejection degrades to liveness-only,
// which we cover in a separate fallback test).
func TestReconcileTerminals_RejectsRecycledPid(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pidStart token is only populated on Linux")
	}
	runPath := t.TempDir()
	// Our own PID is alive; pidStart=1 will not match the real start-time
	// token, so reconcile must treat this as a recycled-PID stranger.
	writeTerminalDocFull(t, runPath, "id-1", api.Ready, os.Getpid(), 1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	docs := []api.TerminalDoc{{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Spec:       api.TerminalSpec{ID: api.ID("id-1"), RunPath: runPath},
		Status: api.TerminalStatus{
			State:           api.Ready,
			Pid:             os.Getpid(),
			PidStart:        1,
			TerminalRunPath: filepath.Join(runPath, defaults.TerminalsRunPath, "id-1"),
		},
	}}
	ReconcileTerminals(context.Background(), logger, runPath, docs)

	if docs[0].Status.State != api.Exited {
		t.Fatalf("in-memory doc: expected state=Exited after reconcile, got %v", docs[0].Status.State)
	}
	if got := readTerminalState(t, runPath, "id-1"); got != api.Exited {
		t.Fatalf("persisted doc: expected state=Exited after reconcile, got %v", got)
	}
}

// TestReconcileTerminals_ZeroTokenFallsBackToLiveness covers legacy metadata
// (or non-Linux platforms) where PidStart is unset: the predicate degrades to
// liveness-only, matching pidutil.Match's documented contract.
func TestReconcileTerminals_ZeroTokenFallsBackToLiveness(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalDocFull(t, runPath, "id-live", api.Ready, os.Getpid(), 0)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	docs := []api.TerminalDoc{{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Spec:       api.TerminalSpec{ID: api.ID("id-live"), RunPath: runPath},
		Status: api.TerminalStatus{
			State:           api.Ready,
			Pid:             os.Getpid(),
			PidStart:        0,
			TerminalRunPath: filepath.Join(runPath, defaults.TerminalsRunPath, "id-live"),
		},
	}}
	ReconcileTerminals(context.Background(), logger, runPath, docs)

	if docs[0].Status.State != api.Ready {
		t.Fatalf("expected state=Ready preserved for live PID with zero token, got %v", docs[0].Status.State)
	}
}

// TestReconcileTerminals_DeadPidIsExited covers the original reconcile
// contract: a dead PID flips state to Exited. Regression coverage in case
// the isInstanceAlive refactor broke the existing path.
func TestReconcileTerminals_DeadPidIsExited(t *testing.T) {
	runPath := t.TempDir()
	const deadPid = 0x7fffffff // very-high PID unlikely to exist
	writeTerminalDocFull(t, runPath, "id-dead", api.Ready, deadPid, 0)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	docs := []api.TerminalDoc{{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Spec:       api.TerminalSpec{ID: api.ID("id-dead"), RunPath: runPath},
		Status: api.TerminalStatus{
			State:           api.Ready,
			Pid:             deadPid,
			TerminalRunPath: filepath.Join(runPath, defaults.TerminalsRunPath, "id-dead"),
		},
	}}
	ReconcileTerminals(context.Background(), logger, runPath, docs)

	if docs[0].Status.State != api.Exited {
		t.Fatalf("expected state=Exited for dead PID, got %v", docs[0].Status.State)
	}
	if got := readTerminalState(t, runPath, "id-dead"); got != api.Exited {
		t.Fatalf("persisted: expected state=Exited, got %v", got)
	}
}
