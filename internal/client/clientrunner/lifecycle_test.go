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
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

func newLifecycleExec(t *testing.T) (*Exec, chan Event) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	evCh := make(chan Event, 4)
	return &Exec{
		id:         api.ID("client-lc"),
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		events:     evCh,
		metadataMu: sync.RWMutex{},
		metadata: api.ClientDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindClient,
			Spec:       api.ClientSpec{ID: api.ID("client-lc"), RunPath: t.TempDir()},
		},
	}, evCh
}

// TestStartTerminalCmd_Success runs a fast-exiting command and confirms the
// reaper goroutine emits EvCmdExited.
func TestStartTerminalCmd_Success(t *testing.T) {
	sr, evCh := newLifecycleExec(t)
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	sr.stdout = devNull
	sr.stderr = devNull

	term := &api.AttachedTerminal{
		Spec:        &api.TerminalSpec{ID: api.ID("term-lc"), EnvInherit: true},
		Command:     "/bin/sh",
		CommandArgs: []string{"-c", "exit 0"},
	}

	if errStart := sr.StartTerminalCmd(term); errStart != nil {
		t.Fatalf("StartTerminalCmd: %v", errStart)
	}

	select {
	case ev := <-evCh:
		if ev.Type != EvCmdExited {
			t.Fatalf("event type = %v; want EvCmdExited", ev.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for EvCmdExited")
	}
}

// TestStartTerminalCmd_MissingCommand covers the validation branch for an
// empty command / nil args.
func TestStartTerminalCmd_MissingCommand(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	if err := sr.StartTerminalCmd(&api.AttachedTerminal{Spec: &api.TerminalSpec{ID: "x"}}); err == nil {
		t.Fatal("StartTerminalCmd with empty command returned nil error")
	}
}

// TestStartTerminalCmd_NoHomeNoInherit covers the branch where env inheritance
// is off and HOME is unset, which must fail.
func TestStartTerminalCmd_NoHomeNoInherit(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	t.Setenv("HOME", "")

	term := &api.AttachedTerminal{
		Spec:        &api.TerminalSpec{ID: api.ID("term-lc"), EnvInherit: false},
		Command:     "/bin/sh",
		CommandArgs: []string{"-c", "exit 0"},
	}
	if err := sr.StartTerminalCmd(term); err == nil {
		t.Fatal("StartTerminalCmd without HOME and no inherit returned nil error")
	}
}

// TestStartTerminalCmd_StartFailure covers the cmd.Start error branch using a
// non-existent executable.
func TestStartTerminalCmd_StartFailure(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	t.Cleanup(func() { _ = devNull.Close() })
	sr.stdout = devNull
	sr.stderr = devNull

	term := &api.AttachedTerminal{
		Spec:        &api.TerminalSpec{ID: api.ID("term-lc"), EnvInherit: true},
		Command:     "/nonexistent/binary-xyz",
		CommandArgs: []string{},
	}
	if err := sr.StartTerminalCmd(term); err == nil {
		t.Fatal("StartTerminalCmd with bad executable returned nil error")
	}
}

// TestClose_RemovesSocketAndUpdatesState verifies Close cancels the context,
// removes the control socket, and drives the metadata state to ClientExited.
func TestClose_RemovesSocketAndUpdatesState(t *testing.T) {
	sr, _ := newLifecycleExec(t)

	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("seed socket file: %v", err)
	}
	sr.metadata.Spec.SockerCtrl = sockPath

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if sr.ctx.Err() == nil {
		t.Error("Close did not cancel the context")
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("control socket still present after Close: stat err=%v", err)
	}
	st, _ := sr.State()
	if *st != api.ClientExited {
		t.Errorf("post-Close state = %v; want ClientExited", *st)
	}
}

// TestDetach_Success covers the happy path: the terminal client detaches and a
// message is written to stdout.
func TestDetach_Success(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	sr.stdout = w

	var gotID api.ID
	sr.terminalClient = &mockTerminalClient{
		detachFunc: func(_ context.Context, id *api.ID) error {
			gotID = *id
			return nil
		},
	}

	// Drain the pipe so the WriteString never blocks on a full buffer.
	go func() {
		buf := make([]byte, 256)
		_, _ = r.Read(buf)
	}()

	if errDetach := sr.Detach(); errDetach != nil {
		t.Fatalf("Detach: %v", errDetach)
	}
	if gotID != sr.id {
		t.Errorf("Detach passed id %q; want %q", gotID, sr.id)
	}
}

// TestDetach_RPCError covers the branch where the terminal client returns an
// error.
func TestDetach_RPCError(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	sr.terminalClient = &mockTerminalClient{
		detachFunc: func(_ context.Context, _ *api.ID) error { return errors.New("rpc fail") },
	}
	if err := sr.Detach(); err == nil {
		t.Fatal("Detach returned nil error when terminal client failed")
	}
}

// TestDetach_StdoutWriteError covers the branch where the detach succeeds at
// the RPC layer but writing the confirmation banner to stdout fails. As of #383
// the banner is a best-effort post-drain message inside restoreParentTerminal,
// not Detach's return value — the parent-terminal restore is non-negotiable and
// a failed banner write must not turn a successful detach into an error.
func TestDetach_StdoutWriteError(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close()
	_ = w.Close() // closed write end -> banner WriteString fails (best-effort, logged)
	sr.stdout = w
	sr.terminalClient = &mockTerminalClient{} // detach succeeds by default

	if errDetach := sr.Detach(); errDetach != nil {
		t.Fatalf("Detach returned %v on banner-write failure; want nil (RPC succeeded, banner is best-effort)", errDetach)
	}
}

// TestResizeAndWaitClose covers the no-op Resize and WaitClose methods.
func TestResizeAndWaitClose(t *testing.T) {
	sr, _ := newLifecycleExec(t)
	sr.Resize(api.ResizeArgs{Cols: 1, Rows: 1}) // no-op, must not panic
	if err := sr.WaitClose(nil); err != nil {
		t.Fatalf("WaitClose: %v", err)
	}
}
