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
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newBareExec builds the smallest Exec for exercising helpers that touch
// metadata and the child-done plumbing without a PTY or RPC. RunPath points at
// a fresh TempDir; callers that need the metadata dir to exist on disk should
// MkdirAll it (or run CreateMetadata) first.
func newBareExec(t *testing.T) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		ctx:           ctx,
		ctxCancel:     cancel,
		logger:        newDiscardLogger(),
		id:            "bare",
		metadata:      api.TerminalDoc{Spec: api.TerminalSpec{ID: "bare", RunPath: t.TempDir()}},
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{},
		closePTY:      &sync.Once{},
		closeCapture:  &sync.Once{},
		closeClosedCh: &sync.Once{},
	}
}

// newLiveSpec returns a TerminalSpec whose log/capture artifacts live under a
// fresh TempDir, wired to run /bin/cat (which idles on the PTY until SIGTERM).
func newLiveSpec(t *testing.T, command string, args ...string) *api.TerminalSpec {
	t.Helper()
	dir := t.TempDir()
	return &api.TerminalSpec{
		ID:            "live",
		Name:          "live",
		Command:       command,
		CommandArgs:   args,
		RunPath:       dir,
		CaptureFile:   filepath.Join(dir, "capture.log"),
		ShutdownGrace: time.Second,
	}
}

// TestStartTerminalFullLifecycle drives StartTerminal → SetupShell →
// OnInitShell → PostAttachShell → Resize → Screenshot → Close end-to-end with
// a real PTY and a /bin/cat child, covering the PTY-lifecycle and teardown
// branches of lifecycle.go and terminal.go.
func TestStartTerminalFullLifecycle(t *testing.T) {
	spec := newLiveSpec(t, "/bin/cat")
	spec.SetPrompt = true // exercise the default-prompt assignment branch
	spec.Stages = api.StagesSpec{
		OnInit:     []api.ExecStep{{Script: "true", Env: map[string]string{"FOO": "bar"}}},
		PostAttach: []api.ExecStep{{Script: "true"}},
	}
	// Set and pre-create a log file so applyLogFilePerms hits its chmod
	// branch rather than the empty-path no-op.
	spec.LogFile = filepath.Join(spec.RunPath, "term.log")
	if err := os.WriteFile(spec.LogFile, nil, 0o600); err != nil {
		t.Fatalf("seed log file: %v", err)
	}

	sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}

	evCh := make(chan Event, 32)
	if err := sr.StartTerminal(evCh); err != nil {
		t.Fatalf("StartTerminal: %v", err)
	}

	if _, err := sr.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := sr.SetupShell(); err != nil {
		t.Fatalf("SetupShell: %v", err)
	}
	if err := sr.OnInitShell(); err != nil {
		t.Fatalf("OnInitShell: %v", err)
	}
	if err := sr.PostAttachShell(); err != nil {
		t.Fatalf("PostAttachShell: %v", err)
	}

	sr.Resize(api.ResizeArgs{Cols: 120, Rows: 40})
	if _, err := sr.Screenshot(&api.ScreenshotArgs{}); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close is idempotent (per-resource sync.Once guards); a second call must
	// not panic on already-closed channels.
	if err := sr.Close(nil); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestStartTerminalEmitsExitEvent uses a fast-exiting child so watchChildExit
// observes cmd.Wait returning and waitOnTerminal forwards EvCmdExited.
func TestStartTerminalEmitsExitEvent(t *testing.T) {
	spec := newLiveSpec(t, "/bin/sh", "-c", "exit 0")

	sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}

	evCh := make(chan Event, 8)
	if err := sr.StartTerminal(evCh); err != nil {
		t.Fatalf("StartTerminal: %v", err)
	}
	t.Cleanup(func() { _ = sr.Close(nil) })

	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-evCh:
			if ev.Type == EvCmdExited {
				return
			}
		case <-deadline:
			t.Fatal("did not receive EvCmdExited within 5s")
		}
	}
}

// TestStartTerminalMissingDir fails fast when the metadata directory does not
// exist: startPty's updateMetadata write errors, so StartTerminal returns.
func TestStartTerminalMissingDir(t *testing.T) {
	spec := newLiveSpec(t, "/bin/cat")
	// RunPath points at a path with no terminals/<id>/ dir created (no
	// CreateMetadata call), so the updateMetadata inside startPty fails.
	sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
	t.Cleanup(func() { _ = sr.Close(nil) })

	evCh := make(chan Event, 4)
	if err := sr.StartTerminal(evCh); err == nil {
		t.Fatal("expected StartTerminal to fail when metadata dir is absent")
	}
}

func TestPrepareTerminalCommand(t *testing.T) {
	t.Run("inherit env", func(t *testing.T) {
		sr := newBareExec(t)
		sr.metadata.Spec.Command = "/bin/true"
		sr.metadata.Spec.EnvInherit = true
		if err := sr.prepareTerminalCommand(); err != nil {
			t.Fatalf("prepareTerminalCommand: %v", err)
		}
		if sr.cmd == nil || sr.cmd.Path != "/bin/true" {
			t.Fatalf("cmd not prepared: %+v", sr.cmd)
		}
		if !hasEnvKey(sr.cmd.Env, "TERM=") {
			t.Error("expected TERM to be defaulted")
		}
	})

	t.Run("home only", func(t *testing.T) {
		t.Setenv("HOME", "/home/tester")
		sr := newBareExec(t)
		sr.metadata.Spec.Command = "/bin/true"
		sr.metadata.Spec.EnvInherit = false
		sr.metadata.Spec.Env = []string{"CUSTOM=1"}
		if err := sr.prepareTerminalCommand(); err != nil {
			t.Fatalf("prepareTerminalCommand: %v", err)
		}
		if !hasEnvKey(sr.cmd.Env, "HOME=/home/tester") {
			t.Error("expected HOME to be propagated")
		}
		if !hasEnvKey(sr.cmd.Env, "CUSTOM=1") {
			t.Error("expected custom env to be propagated")
		}
	})

	t.Run("home unset is fatal", func(t *testing.T) {
		t.Setenv("HOME", "")
		sr := newBareExec(t)
		sr.metadata.Spec.Command = "/bin/true"
		sr.metadata.Spec.EnvInherit = false
		if err := sr.prepareTerminalCommand(); err == nil {
			t.Fatal("expected error when HOME is unset and EnvInherit is false")
		}
	})

	t.Run("preset TERM not overridden", func(t *testing.T) {
		t.Setenv("HOME", "/home/tester")
		sr := newBareExec(t)
		sr.metadata.Spec.Command = "/bin/true"
		sr.metadata.Spec.Env = []string{"TERM=vt100"}
		if err := sr.prepareTerminalCommand(); err != nil {
			t.Fatalf("prepareTerminalCommand: %v", err)
		}
		if hasEnvKey(sr.cmd.Env, "TERM=xterm-256color") {
			t.Error("preset TERM should not be replaced with the default")
		}
	})

	t.Run("cwd expanded", func(t *testing.T) {
		t.Setenv("HOME", "/home/tester")
		dir := t.TempDir()
		sr := newBareExec(t)
		sr.metadata.Spec.Command = "/bin/true"
		sr.metadata.Spec.Cwd = dir
		if err := sr.prepareTerminalCommand(); err != nil {
			t.Fatalf("prepareTerminalCommand: %v", err)
		}
		if sr.cmd.Dir != dir {
			t.Errorf("cmd.Dir = %q, want %q", sr.cmd.Dir, dir)
		}
	})
}

func hasEnvKey(env []string, prefix string) bool {
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func TestApplyLogFilePerms(t *testing.T) {
	t.Run("empty path is a no-op", func(t *testing.T) {
		sr := newBareExec(t)
		sr.metadata.Spec.LogFile = ""
		if err := sr.applyLogFilePerms(); err != nil {
			t.Fatalf("applyLogFilePerms: %v", err)
		}
	})

	t.Run("chmods an existing file", func(t *testing.T) {
		sr := newBareExec(t)
		logFile := filepath.Join(t.TempDir(), "term.log")
		if err := os.WriteFile(logFile, nil, 0o600); err != nil {
			t.Fatalf("seed log file: %v", err)
		}
		sr.metadata.Spec.LogFile = logFile
		sr.metadata.Spec.LogFileMode = 0o640
		if err := sr.applyLogFilePerms(); err != nil {
			t.Fatalf("applyLogFilePerms: %v", err)
		}
		info, err := os.Stat(logFile)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Errorf("mode = %o, want 0640", info.Mode().Perm())
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		sr := newBareExec(t)
		sr.metadata.Spec.LogFile = filepath.Join(t.TempDir(), "absent.log")
		if err := sr.applyLogFilePerms(); err == nil {
			t.Fatal("expected chmod of a missing log file to error")
		}
	})
}

func TestWriteError(t *testing.T) {
	sr := newBareExec(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close() // closing the read end makes writes fail
	_ = w.Close()
	sr.ptmx = w

	n, werr := sr.Write([]byte("abc"))
	if werr == nil {
		t.Fatal("expected Write to fail on a closed pipe")
	}
	if n != 0 {
		t.Errorf("n = %d, want 0 on first-byte failure", n)
	}
}

func TestWriteStage(t *testing.T) {
	t.Run("nil stage", func(t *testing.T) {
		sr := newBareExec(t)
		if err := sr.writeStage(nil); err != nil {
			t.Fatalf("writeStage(nil): %v", err)
		}
	})

	t.Run("writes each step to the pty", func(t *testing.T) {
		sr := newBareExec(t)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		t.Cleanup(func() { _ = w.Close() })
		// Drain the read end so byte-by-byte Write never blocks on a full pipe.
		go func() {
			buf := make([]byte, 256)
			for {
				if _, e := r.Read(buf); e != nil {
					return
				}
			}
		}()
		sr.ptmx = w

		stage := []api.ExecStep{{Script: "echo hi", Env: map[string]string{"K": "v"}}}
		if err := sr.writeStage(&stage); err != nil {
			t.Fatalf("writeStage: %v", err)
		}
	})

	t.Run("write failure surfaces", func(t *testing.T) {
		sr := newBareExec(t)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		_ = r.Close()
		_ = w.Close()
		sr.ptmx = w // closed pipe: the first PTY write fails

		stage := []api.ExecStep{{Script: "echo hi"}}
		if err := sr.writeStage(&stage); err == nil {
			t.Fatal("expected writeStage to surface a PTY write error")
		}
	})
}

// TestShellStateUpdateError covers the early-return error branches of
// SetupShell / OnInitShell: the metadata directory does not exist (no
// CreateMetadata), so the first updateTerminalState write fails.
func TestShellStateUpdateError(t *testing.T) {
	t.Run("SetupShell", func(t *testing.T) {
		sr := newBareExec(t)
		if err := sr.SetupShell(); err == nil {
			t.Fatal("expected SetupShell to fail when metadata cannot be written")
		}
	})

	t.Run("OnInitShell", func(t *testing.T) {
		sr := newBareExec(t)
		if err := sr.OnInitShell(); err == nil {
			t.Fatal("expected OnInitShell to fail when metadata cannot be written")
		}
	})
}

func TestPostAttachShellCtxCancelled(t *testing.T) {
	sr := newBareExec(t)
	// State is never Ready, so PostAttachShell loops until the context is
	// cancelled, then returns ctx.Err().
	sr.metadata.Status.State = api.Starting
	sr.ctxCancel()

	if err := sr.PostAttachShell(); err == nil {
		t.Fatal("expected PostAttachShell to return the context error")
	}
}

func TestChildDoneTransitions(t *testing.T) {
	sr := newBareExec(t)
	if sr.childDone() {
		t.Fatal("childDone should be false before markChildDone")
	}
	if !sr.markChildDone() {
		t.Fatal("first markChildDone should report it fired")
	}
	if !sr.childDone() {
		t.Fatal("childDone should be true after markChildDone")
	}
	if sr.markChildDone() {
		t.Fatal("second markChildDone should be a no-op")
	}
}

func TestID(t *testing.T) {
	sr := newBareExec(t)
	if sr.ID() != "bare" {
		t.Errorf("ID() = %q, want bare", sr.ID())
	}
}

func TestNewTerminalRunnerExec(t *testing.T) {
	spec := &api.TerminalSpec{ID: "ctor", Name: "ctor", RunPath: t.TempDir()}
	tr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec)
	if tr.ID() != "ctor" {
		t.Errorf("ID() = %q, want ctor", tr.ID())
	}
	if _, ok := tr.(*Exec); !ok {
		t.Errorf("expected *Exec, got %T", tr)
	}
}

func TestUseListener(t *testing.T) {
	sr := newBareExec(t)
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	sr.metadata.Spec.SocketMode = 0o660

	if err := sr.UseListener(ln); err != nil {
		t.Fatalf("UseListener: %v", err)
	}
	if sr.lnCtrl != ln {
		t.Error("UseListener did not store the listener")
	}
	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if info.Mode().Perm() != 0o660 {
		t.Errorf("socket mode = %o, want 0660", info.Mode().Perm())
	}
}

func TestAttachDetach(t *testing.T) {
	// Client teardown is asynchronous: Detach spawns a 100ms grace goroutine
	// that closes the server conn, whose reader-EOF path runs cleanupClient ->
	// updateTerminalAttachers -> WriteMetadata after the test body has already
	// returned. WriteMetadata recreates files under <runPath>/terminals/ad, so
	// t.TempDir()'s single-shot RemoveAll can race that late write and fail
	// with "directory not empty" under full-package parallel load (issue
	// #351). This cleanup runs before t.TempDir's (LIFO) and drains the writers
	// via a bounded retry, so t.TempDir's own RemoveAll then finds the tree
	// already gone (a no-op on a missing path).
	runPath := t.TempDir()
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for {
			if rmErr := os.RemoveAll(runPath); rmErr == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("draining run dir %s did not settle within deadline", runPath)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	spec := &api.TerminalSpec{ID: "ad", Name: "ad", RunPath: runPath}
	sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}
	t.Cleanup(func() { _ = sr.Close(nil) })

	// handleClient registers each attacher's writer on the fan-out, so the
	// runner needs a live multiOutW even though no PTY is running.
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.multiOutW = NewDynamicMultiWriter(newDiscardLogger())
	sr.ptyPipesMu.Unlock()

	clientID := api.ID("client-1")
	req := &api.AttachRequest{ClientID: clientID}
	resp := &api.ResponseWithFD{}
	if err := sr.Attach(req, resp); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if len(resp.FDs) != 1 {
		t.Fatalf("expected 1 returned fd, got %d", len(resp.FDs))
	}
	// Release the client-side fd at the end of the test, not now. Closing it
	// makes the server-side reader observe EOF, tripping the handleClient
	// auto-detach path (cleanupClient -> removeClient); closing it before the
	// assertion below lets that cleanup race the read and remove the client
	// first under parallel load (issue #351). Attach already registered the
	// client synchronously (CreateNewClient -> addClient), so the assertion is
	// deterministic while this fd stays open.
	defer func() { _ = syscall.Close(resp.FDs[0]) }()

	if _, ok := sr.getClient(clientID); !ok {
		t.Fatal("client should be registered after Attach")
	}

	if err := sr.Detach(&clientID); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if _, ok := sr.getClient(clientID); ok {
		t.Error("client should be removed after Detach")
	}

	unknown := api.ID("nope")
	if err := sr.Detach(&unknown); err == nil {
		t.Error("Detach of an unknown client should error")
	}
}
