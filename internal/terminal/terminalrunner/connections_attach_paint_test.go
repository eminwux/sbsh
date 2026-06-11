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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

// Default attach (fullCapture=false, clearScreen=false) synthesizes a
// bounded repaint from the live screen model — the current screen, not the
// raw capture file — and never erases the client's terminal.
func TestInitialAttachPaintRepaintsScreen(t *testing.T) {
	sr := newScreenExec()
	screen := newScreenModel()
	feed(screen, "current screen state")
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.screen = screen
	sr.ptyPipesMu.Unlock()

	id := api.ID("c1")
	got := string(sr.initialAttachPaint(&ioClient{id: &id, fullCapture: false}))

	if strings.Contains(got, escClearScreen) {
		t.Errorf("default repaint paint must not clear the screen; got %q", got)
	}
	if !strings.Contains(got, "current screen state") {
		t.Errorf("repaint paint should contain the live screen; got %q", got)
	}
}

// --clear-screen (clearScreen=true) restores the legacy clear-and-repaint
// seed paint.
func TestInitialAttachPaintClearScreen(t *testing.T) {
	sr := newScreenExec()
	screen := newScreenModel()
	feed(screen, "current screen state")
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.screen = screen
	sr.ptyPipesMu.Unlock()

	id := api.ID("c1c")
	got := string(sr.initialAttachPaint(&ioClient{id: &id, fullCapture: false, clearScreen: true}))

	if !strings.Contains(got, escClearScreen) {
		t.Errorf("clear-screen repaint paint should clear the screen; got %q", got)
	}
	if !strings.Contains(got, "current screen state") {
		t.Errorf("repaint paint should contain the live screen; got %q", got)
	}
}

// --full-capture (fullCapture=true) replays the entire raw capture buffer
// verbatim, byte-for-byte, including control sequences and history.
func TestInitialAttachPaintFullCaptureReplaysRaw(t *testing.T) {
	sr := newScreenExec()

	raw := "line one\r\n\x1b[31mcolored\x1b[0m\r\nline three\r\n"
	captureFile := filepath.Join(t.TempDir(), "capture.raw")
	if err := os.WriteFile(captureFile, []byte(raw), 0o600); err != nil {
		t.Fatalf("write capture file: %v", err)
	}
	sr.metadataMu.Lock()
	sr.metadata.Status.CaptureFile = captureFile
	sr.metadataMu.Unlock()

	id := api.ID("c2")
	got := sr.initialAttachPaint(&ioClient{id: &id, fullCapture: true})
	if string(got) != raw {
		t.Errorf("full-capture should replay the raw buffer verbatim; got %q want %q", got, raw)
	}
}

// A missing/unreadable capture on the full-capture path returns no bytes
// (write nothing) rather than denying the attach.
func TestInitialAttachPaintFullCaptureMissingFile(t *testing.T) {
	sr := newScreenExec()
	sr.metadataMu.Lock()
	sr.metadata.Status.CaptureFile = filepath.Join(t.TempDir(), "does-not-exist")
	sr.metadataMu.Unlock()

	id := api.ID("c3")
	if got := sr.initialAttachPaint(&ioClient{id: &id, fullCapture: true}); got != nil {
		t.Errorf("missing capture should yield no bytes; got %q", got)
	}
}

// An absent screen model on the repaint path returns no bytes rather than
// panicking — the attach still proceeds with live output only.
func TestInitialAttachPaintRepaintNoScreen(t *testing.T) {
	sr := newScreenExec() // ptyPipes.screen is nil

	id := api.ID("c4")
	if got := sr.initialAttachPaint(&ioClient{id: &id, fullCapture: false}); got != nil {
		t.Errorf("nil screen model should yield no bytes; got %q", got)
	}
}
