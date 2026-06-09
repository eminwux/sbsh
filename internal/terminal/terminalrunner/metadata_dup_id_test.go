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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func readMetadataDoc(t *testing.T, dir string) api.TerminalDoc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata.json in %q: %v", dir, err)
	}
	var doc api.TerminalDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	return doc
}

// TestCreateMetadata_RefusesLiveOwnerDuplicateID reproduces issue #386 at the
// runner level: a second start reusing a live terminal's --id (with a different
// --name, which sails past the CLI name check) must be refused *without*
// clobbering the live owner's metadata.json. The clobber was the root cause of
// the live terminal going invisible/unaddressable and prune deleting its still-
// running run dir.
func TestCreateMetadata_RefusesLiveOwnerDuplicateID(t *testing.T) {
	runPath := t.TempDir()
	id := api.ID("aaaa1111")
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, string(id))

	// Terminal A: alive (this test process is the recorded owner pid).
	srA := newMetadataExec(t, runPath, "", id)
	srA.metadata.Spec.Name = "terma"
	if err := srA.CreateMetadata(); err != nil {
		t.Fatalf("A CreateMetadata: %v", err)
	}
	before := readMetadataDoc(t, dir)
	if before.Spec.Name != "terma" || before.Status.Pid != os.Getpid() {
		t.Fatalf("A metadata not as expected: name=%q pid=%d", before.Spec.Name, before.Status.Pid)
	}

	// Terminal B: same --id, different --name. Must be refused.
	srB := newMetadataExec(t, runPath, "", id)
	srB.metadata.Spec.Name = "termb"
	err := srB.CreateMetadata()
	if err == nil {
		t.Fatal("B CreateMetadata: expected refusal, got nil")
	}
	if !errors.Is(err, errdefs.ErrTerminalIDInUse) {
		t.Fatalf("B CreateMetadata: expected ErrTerminalIDInUse, got: %v", err)
	}

	// AC: A's metadata.json is intact — name, pid, and state unchanged.
	after := readMetadataDoc(t, dir)
	if after.Spec.Name != "terma" {
		t.Fatalf("A metadata name clobbered: got %q, want terma", after.Spec.Name)
	}
	if after.Status.Pid != before.Status.Pid {
		t.Fatalf("A metadata pid clobbered: got %d, want %d", after.Status.Pid, before.Status.Pid)
	}
	if after.Status.State != before.Status.State {
		t.Fatalf("A metadata state clobbered: got %v, want %v", after.Status.State, before.Status.State)
	}

	// AC: prune-safe. PruneTerminal removes only entries reconciled to Exited;
	// reconciling A's intact (live pid, non-Exited) metadata keeps it active, so
	// prune leaves the running terminal's run dir alone.
	terminals, scanErr := discovery.ScanTerminals(srB.ctx, srB.logger, runPath)
	if scanErr != nil {
		t.Fatalf("ScanTerminals: %v", scanErr)
	}
	discovery.ReconcileTerminals(srB.ctx, srB.logger, runPath, terminals)
	if len(terminals) != 1 {
		t.Fatalf("expected exactly 1 terminal on disk, got %d", len(terminals))
	}
	if terminals[0].Status.State == api.Exited {
		t.Fatal("live terminal reconciled to Exited — prune would delete a running terminal's dir")
	}
}

// TestCreateMetadata_RecyclesExitedOwnerID confirms the guard does not
// over-block: an --id whose prior owner exited cleanly (State==Exited) is free
// to recycle, matching the name-recycling semantics of
// VerifyTerminalNameAvailable.
func TestCreateMetadata_RecyclesExitedOwnerID(t *testing.T) {
	runPath := t.TempDir()
	id := api.ID("bbbb2222")
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, string(id))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	// Pre-seed an exited owner with a live pid: state, not liveness, marks it
	// reusable.
	prior := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Spec:       api.TerminalSpec{ID: id, Name: "old", RunPath: runPath},
		Status:     api.TerminalStatus{State: api.Exited, Pid: os.Getpid(), TerminalRunPath: dir},
	}
	b, err := json.Marshal(prior)
	if err != nil {
		t.Fatalf("marshal prior: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o600); err != nil {
		t.Fatalf("write prior metadata: %v", err)
	}

	sr := newMetadataExec(t, runPath, "", id)
	sr.metadata.Spec.Name = "new"
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata on exited-owner id: expected nil, got: %v", err)
	}
	got := readMetadataDoc(t, dir)
	if got.Status.State != api.Initializing {
		t.Fatalf("recycled metadata state = %v, want Initializing", got.Status.State)
	}
}
