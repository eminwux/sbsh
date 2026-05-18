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

package discovery_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/pkg/api"
)

// deadPid is a very-high PID unlikely to exist; isProcessAlive maps it to
// "process is gone" so reconcile flips state to Exited.
const deadPid = 0x7fffffff

func writeClientDoc(t *testing.T, runPath, id string, state api.ClientStatusMode, pid int) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.ClientsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Spec:       api.ClientSpec{ID: api.ID(id), RunPath: runPath},
		Status: api.ClientStatus{
			State:         state,
			Pid:           pid,
			ClientRunPath: dir,
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

// TestScanAndPruneTerminals_ReconcilesStaleReady asserts a terminal with a
// dead PID + state=Ready on disk is reconciled to Exited and pruned in one
// pass, without requiring a prior sb-ls round trip.
func TestScanAndPruneTerminals_ReconcilesStaleReady(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-stale", "alpha", api.Ready, deadPid)

	termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-stale")
	if _, err := os.Stat(termDir); err != nil {
		t.Fatalf("setup: terminal dir missing before prune: %v", err)
	}

	if err := discovery.ScanAndPruneTerminals(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneTerminals: unexpected error: %v", err)
	}

	if _, err := os.Stat(termDir); !os.IsNotExist(err) {
		t.Fatalf("expected stale terminal dir to be pruned, stat err=%v", err)
	}
}

// TestScanAndPruneClients_ReconcilesStaleReady is the client-side mirror:
// a client with a dead PID + state=Ready on disk is reconciled and pruned in
// one pass.
func TestScanAndPruneClients_ReconcilesStaleReady(t *testing.T) {
	runPath := t.TempDir()
	writeClientDoc(t, runPath, "id-stale", api.ClientReady, deadPid)

	clientDir := filepath.Join(runPath, defaults.ClientsRunPath, "id-stale")
	if _, err := os.Stat(clientDir); err != nil {
		t.Fatalf("setup: client dir missing before prune: %v", err)
	}

	if err := discovery.ScanAndPruneClients(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneClients: unexpected error: %v", err)
	}

	if _, err := os.Stat(clientDir); !os.IsNotExist(err) {
		t.Fatalf("expected stale client dir to be pruned, stat err=%v", err)
	}
}

// TestScanAndPruneTerminals_LeavesLive asserts a terminal whose recorded PID
// is live is not pruned (regression for the reconcile-then-prune ordering).
func TestScanAndPruneTerminals_LeavesLive(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-live", "alpha", api.Ready, os.Getpid())

	termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-live")

	if err := discovery.ScanAndPruneTerminals(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneTerminals: unexpected error: %v", err)
	}

	if _, err := os.Stat(termDir); err != nil {
		t.Fatalf("expected live terminal dir to survive prune, stat err=%v", err)
	}
}
