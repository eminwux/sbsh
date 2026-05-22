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
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

// newMetadataExec builds a minimal Exec wired for metadata writes: a real
// run-path temp dir, an in-memory logger, and the maps/locks the metadata
// helpers touch. It does not create the terminals/<id> subdir — CreateMetadata
// MkdirAlls it, and the error-path tests rely on its absence.
func newMetadataExec(t *testing.T, runPath, metadataDir string, id api.ID) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		id:        id,
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{
				RunPath:     runPath,
				MetadataDir: metadataDir,
			},
		},
		clients: make(map[api.ID]*ioClient),
	}
}

// CreateMetadata on the legacy layout (empty MetadataDir) MkdirAlls
// runPath/terminals/<id>, writes metadata.json there, and seeds the
// Initializing status fields.
func TestCreateMetadata_LegacyLayoutCreatesDir(t *testing.T) {
	runPath := t.TempDir()
	id := api.ID("term-legacy")
	sr := newMetadataExec(t, runPath, "", id)

	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}

	dir := filepath.Join(runPath, defaults.TerminalsRunPath, string(id))
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err != nil {
		t.Fatalf("metadata.json not written to %q: %v", dir, err)
	}
	doc, err := sr.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if doc.Status.State != api.Initializing {
		t.Fatalf("state = %v, want Initializing", doc.Status.State)
	}
	if doc.Status.Pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", doc.Status.Pid, os.Getpid())
	}
	if doc.Status.TerminalRunPath != dir {
		t.Fatalf("TerminalRunPath = %q, want %q", doc.Status.TerminalRunPath, dir)
	}
	if doc.Metadata.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
}

// CreateMetadata with a MetadataDir override skips MkdirAll and writes
// metadata.json directly into the embedder-owned directory.
func TestCreateMetadata_EmbedderOwnedDirSkipsMkdir(t *testing.T) {
	runPath := t.TempDir()
	override := t.TempDir() // pre-allocated by the "embedder"
	sr := newMetadataExec(t, runPath, override, api.ID("term-embed"))

	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}

	if _, err := os.Stat(filepath.Join(override, "metadata.json")); err != nil {
		t.Fatalf("metadata.json not written to override dir %q: %v", override, err)
	}
	// The legacy nested layout must not have been created.
	if _, err := os.Stat(filepath.Join(runPath, defaults.TerminalsRunPath)); !os.IsNotExist(err) {
		t.Fatalf("embedder-owned path must skip the terminals/ subdir, stat err = %v", err)
	}
}

// CreateMetadata surfaces the MkdirAll error when the resolved parent is a
// regular file rather than a directory.
func TestCreateMetadata_MkdirFails(t *testing.T) {
	// A file standing where runPath should be makes MkdirAll(runPath/terminals/<id>) fail.
	fileAsRunPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileAsRunPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	sr := newMetadataExec(t, fileAsRunPath, "", api.ID("term-mkdir-fail"))

	if err := sr.CreateMetadata(); err == nil {
		t.Fatal("CreateMetadata: want mkdir error, got nil")
	}
}

// getTerminalDir resolves to the legacy layout for an empty override.
func TestGetTerminalDir_LegacyLayout(t *testing.T) {
	runPath := "/run/sbsh"
	id := api.ID("term-dir")
	sr := newMetadataExec(t, runPath, "", id)

	want := filepath.Join(runPath, defaults.TerminalsRunPath, string(id))
	if got := sr.getTerminalDir(); got != want {
		t.Fatalf("getTerminalDir = %q, want %q", got, want)
	}
}

// Metadata returns a copy: mutating the returned doc does not bleed into the
// runner's protected state.
func TestMetadata_ReturnsCopy(t *testing.T) {
	sr := newMetadataExec(t, t.TempDir(), "", api.ID("term-copy"))
	sr.metadata.Status.State = api.Ready

	doc, err := sr.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	doc.Status.State = api.Exited

	if sr.metadata.Status.State != api.Ready {
		t.Fatalf("internal state mutated through returned copy: %v", sr.metadata.Status.State)
	}
}

// updateTerminalAttachers records the current client IDs and bumps
// LastAttachedAt when the attacher count grows.
func TestUpdateTerminalAttachers_RecordsClientsAndStampsTime(t *testing.T) {
	runPath := t.TempDir()
	id := api.ID("term-attach")
	sr := newMetadataExec(t, runPath, "", id)
	// updateMetadata writes into the legacy dir; create it so the write succeeds.
	if err := os.MkdirAll(filepath.Join(runPath, defaults.TerminalsRunPath, string(id)), 0o700); err != nil {
		t.Fatalf("mkdir terminal dir: %v", err)
	}

	cid := api.ID("client-1")
	sr.addClient(&ioClient{id: &cid})

	if err := sr.updateTerminalAttachers(); err != nil {
		t.Fatalf("updateTerminalAttachers: %v", err)
	}

	doc, _ := sr.Metadata()
	if len(doc.Status.Attachers) != 1 || doc.Status.Attachers[0] != string(cid) {
		t.Fatalf("Attachers = %v, want [%s]", doc.Status.Attachers, cid)
	}
	if doc.Status.LastAttachedAt.IsZero() {
		t.Fatal("LastAttachedAt should be stamped when the attacher count grows")
	}
}

// updateTerminalAttachers skips a nil client id without recording it.
func TestUpdateTerminalAttachers_SkipsNilClientID(t *testing.T) {
	runPath := t.TempDir()
	id := api.ID("term-attach-nil")
	sr := newMetadataExec(t, runPath, "", id)
	if err := os.MkdirAll(filepath.Join(runPath, defaults.TerminalsRunPath, string(id)), 0o700); err != nil {
		t.Fatalf("mkdir terminal dir: %v", err)
	}
	sr.clients[api.ID("ghost")] = &ioClient{id: nil}

	if err := sr.updateTerminalAttachers(); err != nil {
		t.Fatalf("updateTerminalAttachers: %v", err)
	}
	doc, _ := sr.Metadata()
	if len(doc.Status.Attachers) != 0 {
		t.Fatalf("Attachers = %v, want empty (nil id skipped)", doc.Status.Attachers)
	}
}

// updateTerminalState writes the new state through to disk on the happy path.
func TestUpdateTerminalState_Success(t *testing.T) {
	runPath := t.TempDir()
	id := api.ID("term-state")
	sr := newMetadataExec(t, runPath, "", id)
	if err := os.MkdirAll(filepath.Join(runPath, defaults.TerminalsRunPath, string(id)), 0o700); err != nil {
		t.Fatalf("mkdir terminal dir: %v", err)
	}

	if err := sr.updateTerminalState(api.Ready); err != nil {
		t.Fatalf("updateTerminalState: %v", err)
	}
	doc, _ := sr.Metadata()
	if doc.Status.State != api.Ready {
		t.Fatalf("state = %v, want Ready", doc.Status.State)
	}
}

// updateTerminalState surfaces (and warns on) the metadata-write failure when
// the target directory does not exist.
func TestUpdateTerminalState_WriteFailure(t *testing.T) {
	// No terminals/<id> dir is created, so updateMetadata's write fails.
	sr := newMetadataExec(t, t.TempDir(), "", api.ID("term-state-fail"))

	if err := sr.updateTerminalState(api.Ready); err == nil {
		t.Fatal("updateTerminalState: want write error on missing dir, got nil")
	}
}

// trySendEvent delivers on a buffered channel and silently drops when the
// channel is full, so the PTY reader never blocks on a busy controller.
func TestTrySendEvent_DeliverAndDrop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := make(chan Event, 1)

	ev := Event{ID: api.ID("t"), Type: EvCmdExited, When: time.Now()}
	trySendEvent(logger, ch, ev)
	select {
	case got := <-ch:
		if got.Type != EvCmdExited {
			t.Fatalf("event type = %v, want EvCmdExited", got.Type)
		}
	default:
		t.Fatal("expected the event to be delivered on the buffered channel")
	}

	// Fill the buffer, then a second send must drop rather than block.
	trySendEvent(logger, ch, ev)
	trySendEvent(logger, ch, Event{ID: api.ID("t"), Type: EvError, Err: errors.New("boom")})
	if got := <-ch; got.Type != EvCmdExited {
		t.Fatalf("buffered event = %v, want the first (EvCmdExited); the overflow send should have dropped", got.Type)
	}
	select {
	case extra := <-ch:
		t.Fatalf("expected the overflow event to be dropped, got %v", extra.Type)
	default:
	}
}

// newCaptureWriter fails when the canonical path cannot be opened (parent
// directory does not exist).
func TestNewCaptureWriter_OpenFailure(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "missing-dir", "capture")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := newCaptureWriter(canonical, logger, nil, captureFormatOpts{}); err == nil {
		t.Fatal("newCaptureWriter: want open error for missing parent dir, got nil")
	}
}

// newCaptureWriter propagates an applyPerms failure on the freshly opened
// canonical and closes the fd it just opened.
func TestNewCaptureWriter_ApplyPermsFailure(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	permErr := errors.New("perm denied")

	_, err := newCaptureWriter(canonical, logger, func(string) error { return permErr }, captureFormatOpts{})
	if !errors.Is(err, permErr) {
		t.Fatalf("newCaptureWriter err = %v, want %v", err, permErr)
	}
}

// Writing after Close returns os.ErrClosed via the nil-fd guard.
func TestCaptureWriter_WriteAfterCloseIsErrClosed(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	w := newTestCaptureWriter(t, canonical, 1<<20, 8, 1<<30)
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := w.Write([]byte("late")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Write after Close err = %v, want os.ErrClosed", err)
	}
	// A second Close is a no-op.
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// A raw-mode write reports the underlying file error (and partial count) when
// the live-segment fd has been closed out from under the writer.
func TestCaptureWriter_RawWriteError(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	w := newTestCaptureWriter(t, canonical, 1<<20, 8, 1<<30)
	// Close the underlying fd directly, leaving w.f non-nil so Write reaches
	// the file-write error branch rather than the nil-fd guard.
	if err := w.f.Close(); err != nil {
		t.Fatalf("close underlying fd: %v", err)
	}

	if _, err := w.Write([]byte("data")); err == nil {
		t.Fatal("raw Write: want underlying write error, got nil")
	}
	w.f = nil // prevent the helper's deferred Close from double-closing
}

// An asciicast-mode write reports nothing durably recorded (n=0) on a
// file-write failure, since a partial event line is unusable.
func TestCaptureWriter_AsciicastWriteError(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	w := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
	if err := w.f.Close(); err != nil {
		t.Fatalf("close underlying fd: %v", err)
	}

	n, err := w.Write([]byte("data"))
	if err == nil {
		t.Fatal("asciicast Write: want underlying write error, got nil")
	}
	if n != 0 {
		t.Fatalf("asciicast Write n = %d, want 0 on partial-event failure", n)
	}
	w.f = nil
}

// When the live segment cannot be renamed (read-only parent dir), rotation
// fails but reopenCanonical reopens the still-present canonical, so the write
// swallows the error (logs a warning) and capture degrades to "rotation
// skipped this round" rather than stopping.
func TestCaptureWriter_RotateRenameFailureDegrades(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	w := newTestCaptureWriter(t, canonical, 4, 8, 1<<30) // tiny threshold

	// Drop write permission on the parent so os.Rename fails (the canonical
	// file itself stays openable, so reopen succeeds and capture continues).
	// Restore in cleanup so TempDir teardown can rm it.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	// Crossing the threshold triggers rotate(): close succeeds, rename fails,
	// reopenCanonical succeeds on the still-present canonical. Write swallows
	// the rotation error (logs a warning) and still reports bytes consumed.
	if n, err := w.Write([]byte("AAAA")); err != nil || n != 4 {
		t.Fatalf("Write = (%d, %v), want (4, nil)", n, err)
	}
	if w.f == nil {
		t.Fatal("capture should continue on the reopened canonical after a failed rename")
	}
	// A follow-up write still lands rather than failing.
	if _, err := w.Write([]byte("B")); err != nil {
		t.Fatalf("Write after degraded rotation: %v", err)
	}
}

// A failing applyPerms on segment reopen is best-effort: rotation still
// completes and capture continues on the fresh live segment.
func TestCaptureWriter_ReopenApplyPermsFailureIsNonFatal(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var mu sync.Mutex
	calls := 0
	applyPerms := func(string) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		// Succeed on the construction-time open; fail on every reopen so the
		// best-effort warn branch in reopenCanonical runs.
		if calls == 1 {
			return nil
		}
		return errors.New("reopen perm denied")
	}

	w, err := newCaptureWriter(canonical, logger, applyPerms, captureFormatOpts{})
	if err != nil {
		t.Fatalf("newCaptureWriter: %v", err)
	}
	w.maxBytes = 4 // tiny threshold so the first write rotates

	// This write crosses the threshold and rotates; reopenCanonical's applyPerms
	// fails but rotation must not error out.
	if _, err := w.Write([]byte("AAAA")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Capture still works: a follow-up write lands on the fresh live segment.
	if _, err := w.Write([]byte("B")); err != nil {
		t.Fatalf("Write after rotation: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
