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

package clientrunner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

// metadataExecID is the fixed client ID used by the metadata-helper tests.
const metadataExecID = api.ID("client-meta")

// newMetadataExec builds an Exec rooted at a temp run path with a known ID,
// suitable for exercising the metadata read/write helpers. It returns the
// runner and its run path.
func newMetadataExec(t *testing.T) (*Exec, string) {
	t.Helper()
	id := metadataExecID
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runPath := t.TempDir()
	return &Exec{
		id:         id,
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		metadataMu: sync.RWMutex{},
		metadata: api.ClientDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindClient,
			Metadata:   api.ClientMetadata{Name: "test-client"},
			Spec:       api.ClientSpec{ID: id, RunPath: runPath},
		},
	}, runPath
}

// TestCreateMetadata_WritesFileAndState verifies CreateMetadata creates the
// per-client run dir, transitions state to Initializing, records the pid, and
// persists a readable metadata.json.
func TestCreateMetadata_WritesFileAndState(t *testing.T) {
	sr, runPath := newMetadataExec(t)

	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}

	dir := filepath.Join(runPath, defaults.ClientsRunPath, "client-meta")
	metaPath := filepath.Join(dir, "metadata.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	var doc api.ClientDoc
	if errUnmarshal := json.Unmarshal(raw, &doc); errUnmarshal != nil {
		t.Fatalf("unmarshal metadata.json: %v", errUnmarshal)
	}
	if doc.Status.State != api.ClientInitializing {
		t.Errorf("persisted state = %v; want ClientInitializing", doc.Status.State)
	}
	if doc.Status.Pid != os.Getpid() {
		t.Errorf("persisted pid = %d; want %d", doc.Status.Pid, os.Getpid())
	}
	if doc.Status.ClientRunPath != dir {
		t.Errorf("ClientRunPath = %q; want %q", doc.Status.ClientRunPath, dir)
	}
}

// TestCreateMetadata_MkdirFailure forces the MkdirAll to fail by rooting the
// run path under a regular file, exercising the error branch.
func TestCreateMetadata_MkdirFailure(t *testing.T) {
	sr, runPath := newMetadataExec(t)

	// Replace the run dir with a regular file so MkdirAll under it fails.
	blocker := filepath.Join(runPath, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	sr.metadata.Spec.RunPath = blocker

	if err := sr.CreateMetadata(); err == nil {
		t.Fatal("CreateMetadata succeeded; want mkdir error")
	}
}

// TestMetadata_ReturnsDeepCopy verifies Metadata returns a snapshot that
// callers can mutate without racing the runner's own writes.
func TestMetadata_ReturnsDeepCopy(t *testing.T) {
	sr, _ := newMetadataExec(t)
	sr.metadata.Status.State = api.ClientReady

	doc, err := sr.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if doc.Status.State != api.ClientReady {
		t.Fatalf("snapshot state = %v; want ClientReady", doc.Status.State)
	}
	doc.Metadata.Name = "mutated"
	if sr.metadata.Metadata.Name == "mutated" {
		t.Fatal("Metadata returned a shared reference, not a copy")
	}
}

// TestState_ReturnsCurrentState verifies State reflects the current lifecycle
// state.
func TestState_ReturnsCurrentState(t *testing.T) {
	sr, _ := newMetadataExec(t)
	sr.metadata.Status.State = api.ClientAttached

	st, err := sr.State()
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if *st != api.ClientAttached {
		t.Fatalf("State = %v; want ClientAttached", *st)
	}
}

// TestGetTerminalState_NilClient covers the guard returning an error when no
// terminal client has been dialled yet.
func TestGetTerminalState_NilClient(t *testing.T) {
	sr, _ := newMetadataExec(t)
	if _, err := sr.getTerminalState(); err == nil {
		t.Fatal("getTerminalState with nil client returned nil error")
	}
}

// TestGetTerminalState_PropagatesRPCError covers the RPC-failure branch.
func TestGetTerminalState_PropagatesRPCError(t *testing.T) {
	sr, _ := newMetadataExec(t)
	wantErr := errors.New("rpc down")
	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, _ *api.TerminalStatusMode) error { return wantErr },
	}
	if _, err := sr.getTerminalState(); err == nil {
		t.Fatal("getTerminalState returned nil error on RPC failure")
	}
}

// TestGetTerminalMetadata covers both the nil-client guard and the success
// path of the (currently internal) metadata fetch helper.
func TestGetTerminalMetadata(t *testing.T) {
	sr, _ := newMetadataExec(t)

	if _, err := sr.getTerminalMetadata(); err == nil {
		t.Fatal("getTerminalMetadata with nil client returned nil error")
	}

	sr.terminalClient = &mockTerminalClient{
		metadataFunc: func(_ context.Context, md *api.TerminalDoc) error {
			md.Metadata.Name = "term-1"
			return nil
		},
	}
	md, err := sr.getTerminalMetadata()
	if err != nil {
		t.Fatalf("getTerminalMetadata: %v", err)
	}
	if md.Metadata.Name != "term-1" {
		t.Fatalf("metadata name = %q; want term-1", md.Metadata.Name)
	}

	// RPC-failure branch.
	sr.terminalClient = &mockTerminalClient{
		metadataFunc: func(_ context.Context, _ *api.TerminalDoc) error { return errors.New("boom") },
	}
	if _, errRPC := sr.getTerminalMetadata(); errRPC == nil {
		t.Fatal("getTerminalMetadata returned nil error on RPC failure")
	}
}
