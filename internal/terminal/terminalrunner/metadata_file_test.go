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
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func newMetadataFileExec(t *testing.T, dir, file string, id api.ID) *Exec {
	t.Helper()
	sr := newMetadataExec(t, t.TempDir(), dir, id)
	sr.metadata.Spec.ID = id
	sr.metadata.Spec.MetadataFile = file
	return sr
}

func readDocAt(t *testing.T, path string) api.TerminalDoc {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc api.TerminalDoc
	if uerr := json.Unmarshal(b, &doc); uerr != nil {
		t.Fatalf("unmarshal %s: %v", path, uerr)
	}
	return doc
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestMetadataFile_EveryWritePathUsesConfiguredName drives each metadata
// write path (create, state transition, attacher update, close) with
// MetadataFile terminal.json and asserts every one lands on terminal.json,
// and that no metadata.json or stray temp file is left in the directory.
func TestMetadataFile_EveryWritePathUsesConfiguredName(t *testing.T) {
	dir := t.TempDir()
	sr := newMetadataFileExec(t, dir, "terminal.json", "term-custom")
	path := filepath.Join(dir, "terminal.json")

	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}
	if got := readDocAt(t, path).Status.State; got != api.Initializing {
		t.Fatalf("after CreateMetadata state = %v, want Initializing", got)
	}

	if err := sr.updateTerminalState(api.Ready); err != nil {
		t.Fatalf("updateTerminalState: %v", err)
	}
	if got := readDocAt(t, path).Status.State; got != api.Ready {
		t.Fatalf("after updateTerminalState state = %v, want Ready", got)
	}

	cid := api.ID("client-1")
	sr.clients[cid] = &ioClient{id: &cid}
	if err := sr.updateTerminalAttachers(); err != nil {
		t.Fatalf("updateTerminalAttachers: %v", err)
	}
	if got := readDocAt(t, path).Status.Attachers; len(got) != 1 || got[0] != "client-1" {
		t.Fatalf("after updateTerminalAttachers attachers = %v, want [client-1]", got)
	}

	if err := sr.markExited(); err != nil {
		t.Fatalf("markExited: %v", err)
	}
	if got := readDocAt(t, path).Status.State; got != api.Exited {
		t.Fatalf("after markExited state = %v, want Exited", got)
	}

	if got := dirEntries(t, dir); len(got) != 1 || got[0] != "terminal.json" {
		t.Fatalf("metadata dir entries = %v, want only terminal.json", got)
	}
}

// TestMetadataFile_LiveOwnerGuardReadsConfiguredName asserts the #386
// live-owner guard reads the configured file: a second runner reusing a
// live terminal's MetadataDir and MetadataFile is refused, and the live
// owner's terminal.json is left intact.
func TestMetadataFile_LiveOwnerGuardReadsConfiguredName(t *testing.T) {
	dir := t.TempDir()
	srA := newMetadataFileExec(t, dir, "terminal.json", "term-a")
	if err := srA.CreateMetadata(); err != nil {
		t.Fatalf("A CreateMetadata: %v", err)
	}

	srB := newMetadataFileExec(t, dir, "terminal.json", "term-b")
	err := srB.CreateMetadata()
	if !errors.Is(err, errdefs.ErrTerminalIDInUse) {
		t.Fatalf("B CreateMetadata err = %v, want ErrTerminalIDInUse", err)
	}
	if got := readDocAt(t, filepath.Join(dir, "terminal.json")).Spec.ID; got != "term-a" {
		t.Fatalf("terminal.json owner = %q after refused start, want term-a", got)
	}

	// The default name in the same directory is a different document, so
	// it does not see terminal.json's owner.
	srC := newMetadataFileExec(t, dir, "", "term-c")
	if cerr := srC.CreateMetadata(); cerr != nil {
		t.Fatalf("C CreateMetadata (default name): %v", cerr)
	}
}

// TestMetadataFile_InvalidNameRejectedAndNeverWritten asserts CreateMetadata
// rejects an invalid name and that the close-time write does not create a
// file from it either.
func TestMetadataFile_InvalidNameRejectedAndNeverWritten(t *testing.T) {
	for _, name := range []string{"a/b", "../x", ".", ".."} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			assertInvalidNameNeverWritten(t, name)
		})
	}
}

func assertInvalidNameNeverWritten(t *testing.T, name string) {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, "meta")
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	sr := newMetadataFileExec(t, dir, name, "term-bad")
	if err := sr.CreateMetadata(); err == nil {
		t.Fatalf("CreateMetadata with MetadataFile %q: want error", name)
	}
	if err := sr.markExited(); err == nil {
		t.Fatalf("markExited with MetadataFile %q: want error", name)
	}
	if got := dirEntries(t, dir); len(got) != 1 || got[0] != "a" {
		t.Fatalf("metadata dir entries = %v, want only the pre-created a/", got)
	}
	if got := dirEntries(t, filepath.Join(dir, "a")); len(got) != 0 {
		t.Fatalf("a/ entries = %v, want none", got)
	}
	if got := dirEntries(t, parent); len(got) != 1 {
		t.Fatalf("parent entries = %v, want only meta/", got)
	}
}

// TestMetadataFile_CustomNameRequiresMetadataDir asserts a non-default name
// is rejected on the legacy layout, where discovery would prune the
// directory as an orphan, and that no terminal directory is created.
func TestMetadataFile_CustomNameRequiresMetadataDir(t *testing.T) {
	runPath := t.TempDir()
	sr := newMetadataExec(t, runPath, "", "term-legacy-custom")
	sr.metadata.Spec.MetadataFile = "terminal.json"
	if err := sr.CreateMetadata(); err == nil {
		t.Fatal("CreateMetadata with custom name and no MetadataDir: want error")
	}
	if got := dirEntries(t, runPath); len(got) != 0 {
		t.Fatalf("runPath entries = %v, want none", got)
	}
}
