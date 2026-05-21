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

//go:build unix

package spawn

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// sleeperBinary writes an executable shell script that sleeps regardless of
// the argv the handle constructs (NewClient/NewTerminal append their own
// attach/terminal args, so a real /bin/sleep would reject them and exit). It
// stands in for a long-lived sb child whose control socket never appears.
func sleeperBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sleeper.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatalf("write sleeper: %v", err)
	}
	return path
}

// TestClientHandle_Close_NoSocketEscalates spawns a long-lived child whose
// control socket never appears, so sendStopRPC's os.Stat fails and Close must
// escalate through SIGTERM to reap the process.
func TestClientHandle_Close_NoSocketEscalates(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	doc := validAttachDoc(tmp, filepath.Join(tmp, "c.sock"))

	h, err := NewClient(context.Background(), doc, ClientOptions{
		BinaryPath:      sleeperBinary(t),
		StopGracePeriod: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// sendStopRPC against a missing socket should surface the stat error.
	if statErr := h.sendStopRPC(context.Background()); statErr == nil {
		t.Fatal("sendStopRPC: expected error for missing socket")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	_ = h.Close(ctx)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Close took %s; expected prompt escalation", elapsed)
	}
	if !h.proc.exited() {
		t.Fatal("process still alive after Close")
	}
}

// TestTerminalHandle_Close_NoSocketEscalates mirrors the client test on the
// terminal handle.
func TestTerminalHandle_Close_NoSocketEscalates(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	spec := &api.TerminalSpec{SocketFile: filepath.Join(tmp, "socket")}

	h, err := NewTerminal(context.Background(), spec, TerminalOptions{
		BinaryPath:      sleeperBinary(t),
		StopGracePeriod: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTerminal: %v", err)
	}

	if statErr := h.sendStopRPC(context.Background()); statErr == nil {
		t.Fatal("sendStopRPC: expected error for missing socket")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = h.Close(ctx)
	if !h.proc.exited() {
		t.Fatal("process still alive after Close")
	}
}

// TestClientHandle_WaitReady_Timeout covers the ReadyTimeout path: the child
// stays alive and never creates the socket, so WaitReady returns
// ErrReadyTimeout once the internal cap elapses.
func TestClientHandle_WaitReady_Timeout(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	doc := validAttachDoc(tmp, filepath.Join(tmp, "c.sock"))

	h, err := NewClient(context.Background(), doc, ClientOptions{
		BinaryPath:        sleeperBinary(t),
		ReadyTimeout:      150 * time.Millisecond,
		ReadyPollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Close(ctx)
	}()

	err = h.WaitReady(context.Background())
	if !errors.Is(err, ErrReadyTimeout) {
		t.Fatalf("WaitReady err = %v; want ErrReadyTimeout", err)
	}
}

// TestTerminalHandle_WaitReady_Timeout mirrors the client timeout test.
func TestTerminalHandle_WaitReady_Timeout(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	spec := &api.TerminalSpec{SocketFile: filepath.Join(tmp, "socket")}

	h, err := NewTerminal(context.Background(), spec, TerminalOptions{
		BinaryPath:        sleeperBinary(t),
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReadyTimeout:      150 * time.Millisecond,
		ReadyPollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTerminal: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Close(ctx)
	}()

	err = h.WaitReady(context.Background())
	if !errors.Is(err, ErrReadyTimeout) {
		t.Fatalf("WaitReady err = %v; want ErrReadyTimeout", err)
	}
}

// TestClientHandle_WaitClose_ContextCanceled covers the ctx cancellation leg
// of WaitClose: the child stays alive, the caller's context expires.
func TestClientHandle_WaitClose_ContextCanceled(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	doc := validAttachDoc(tmp, filepath.Join(tmp, "c.sock"))

	h, err := NewClient(context.Background(), doc, ClientOptions{
		BinaryPath: sleeperBinary(t),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Close(ctx)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if waitErr := h.WaitClose(ctx); !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("WaitClose err = %v; want context.DeadlineExceeded", waitErr)
	}
}

// TestSignal_AfterExitIsNoop covers process.signal's already-exited fast path.
func TestSignal_AfterExitIsNoop(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("/bin/true")
	proc, err := startProcess(cmd, nil)
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if waitErr := proc.waitExit(ctx); waitErr != nil {
		t.Fatalf("waitExit: %v", waitErr)
	}
	if sigErr := proc.signal(syscall.SIGTERM); sigErr != nil {
		t.Fatalf("signal after exit = %v; want nil", sigErr)
	}
}

// TestProcessPID_ZeroWhenUnstarted covers pid()'s nil-guard fast paths.
func TestProcessPID_ZeroWhenUnstarted(t *testing.T) {
	t.Parallel()
	var nilProc *process
	if got := nilProc.pid(); got != 0 {
		t.Fatalf("nil process pid = %d; want 0", got)
	}
	if got := (&process{}).pid(); got != 0 {
		t.Fatalf("process with nil cmd pid = %d; want 0", got)
	}
}

// TestResolveStdio_ExplicitStreamsNotRedirected covers the else branches of
// resolveStdio where the caller supplies its own streams (no /dev/null
// fallback): NewClient is driven with concrete stdin/stdout/stderr.
func TestResolveStdio_ExplicitStreamsNotRedirected(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	doc := validAttachDoc(tmp, filepath.Join(tmp, "c.sock"))

	var out, errOut bytes.Buffer
	h, err := NewClient(context.Background(), doc, ClientOptions{
		BinaryPath: "/bin/true",
		Stdin:      strings.NewReader(""),
		Stdout:     &out,
		Stderr:     &errOut,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.WaitClose(ctx)
}

// TestGracefulShutdown_SIGKILLEscalation covers the final escalation step: the
// child traps SIGTERM and ignores it, so gracefulShutdown must fall through to
// SIGKILL to reap it.
func TestGracefulShutdown_SIGKILLEscalation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trap.sh")
	// Trap SIGTERM so the SIGTERM step times out and SIGKILL is required.
	script := "#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 1; done\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write trap script: %v", err)
	}
	cmd := exec.Command(path)
	proc, err := startProcess(cmd, nil)
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = proc.gracefulShutdown(ctx, 200*time.Millisecond, nil)
	if !proc.exited() {
		t.Fatal("process still alive after SIGKILL escalation")
	}
}

// TestGracefulShutdown_CtxCanceledMidWait covers the ctx.Err() early return:
// the caller's context is already canceled when gracefulShutdown waits.
func TestGracefulShutdown_CtxCanceledMidWait(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("/bin/sleep", "30")
	proc, err := startProcess(cmd, nil)
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	defer func() {
		_ = proc.signal(syscall.SIGKILL)
		_ = proc.waitExit(context.Background())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	if shutdownErr := proc.gracefulShutdown(ctx, time.Second, nil); !errors.Is(shutdownErr, context.Canceled) {
		t.Fatalf("gracefulShutdown err = %v; want context.Canceled", shutdownErr)
	}
}
