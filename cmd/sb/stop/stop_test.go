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

package stop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func stopTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func Test_NewStopCmd_Metadata(t *testing.T) {
	cmd := NewStopCmd()
	if cmd == nil {
		t.Fatal("NewStopCmd returned nil")
	}
	if cmd.Use != "stop" {
		t.Errorf("Use = %q", cmd.Use)
	}
	// The default RunE prints help; exercise it for coverage.
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Errorf("stop RunE (help) returned error: %v", err)
	}
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "terminal" {
			found = true
		}
	}
	if !found {
		t.Error("stop command missing 'terminal' subcommand")
	}
}

func Test_StopTerminals_RunE_PropagatesError(t *testing.T) {
	viper.Reset()
	cmd := NewStopTerminalsCmd() // binds flags into viper
	viper.Set(config.SB_STOP_TIMEOUT.ViperKey, "5s")
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, t.TempDir())
	ctx := context.WithValue(context.Background(), types.CtxLogger, stopTestLogger())
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{"ghost"})
	if !errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("expected ErrTerminalNotFound from RunE, got: %v", err)
	}
}

func Test_RunStopTerminals_NoActiveTargets(t *testing.T) {
	opts := stopOpts{runPath: t.TempDir(), all: true, timeout: defaultStopTimeout}
	var out, errOut bytes.Buffer
	if err := runStopTerminals(context.Background(), stopTestLogger(), &out, &errOut, opts); err != nil {
		t.Fatalf("runStopTerminals: %v", err)
	}
	if !strings.Contains(out.String(), "no active terminals found") {
		t.Errorf("stdout = %q, want 'no active terminals found'", out.String())
	}
}

func Test_RunStopTerminals_NotFoundIgnored(t *testing.T) {
	opts := stopOpts{runPath: t.TempDir(), name: "ghost", ignoreNotFound: true, timeout: defaultStopTimeout}
	var out, errOut bytes.Buffer
	if err := runStopTerminals(context.Background(), stopTestLogger(), &out, &errOut, opts); err != nil {
		t.Fatalf("expected nil with --ignore-not-found, got: %v", err)
	}
	if !strings.Contains(out.String(), "not found (ignored)") {
		t.Errorf("stdout = %q, want 'not found (ignored)'", out.String())
	}
}

func Test_RunStopTerminals_NotFoundErrors(t *testing.T) {
	opts := stopOpts{runPath: t.TempDir(), name: "ghost", timeout: defaultStopTimeout}
	var out, errOut bytes.Buffer
	err := runStopTerminals(context.Background(), stopTestLogger(), &out, &errOut, opts)
	if !errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("expected ErrTerminalNotFound, got: %v", err)
	}
}

func Test_RunStopTerminals_AlreadyExited(t *testing.T) {
	const deadPid = 0x7fffffff
	runPath := t.TempDir()
	writeStopTestTerminal(t, runPath, "id-dead", "alpha", api.Ready, deadPid)

	opts := stopOpts{runPath: runPath, name: "alpha", timeout: defaultStopTimeout}
	var out, errOut bytes.Buffer
	if err := runStopTerminals(context.Background(), stopTestLogger(), &out, &errOut, opts); err != nil {
		t.Fatalf("runStopTerminals: %v", err)
	}
	if !strings.Contains(out.String(), "already exited") {
		t.Errorf("stdout = %q, want 'already exited'", out.String())
	}
}

func Test_RunStopTerminals_GracefulSIGTERM(t *testing.T) {
	// A real child the SIGTERM fallback can reap. No socket is recorded, so
	// stopViaRPC fails fast and runStopTerminals falls back to SIGTERM.
	child := exec.Command("/bin/sh", "-c", "sleep 60")
	_ = startBackgroundProc(t, child)

	runPath := t.TempDir()
	writeStopTestTerminal(t, runPath, "id-live", "beta", api.Ready, child.Process.Pid)

	opts := stopOpts{runPath: runPath, name: "beta", timeout: 3 * time.Second}
	var out, errOut bytes.Buffer
	if err := runStopTerminals(context.Background(), stopTestLogger(), &out, &errOut, opts); err != nil {
		t.Fatalf("runStopTerminals: %v\nstderr=%q", err, errOut.String())
	}
	if !strings.Contains(out.String(), `terminal "beta" stopped`) {
		t.Errorf("stdout = %q, want 'terminal \"beta\" stopped'", out.String())
	}
}

func Test_ResolveTargets_All_FiltersExited(t *testing.T) {
	runPath := t.TempDir()
	// Exited terminal is filtered out; a live one (self) stays active.
	writeStopTestTerminal(t, runPath, "id-exited", "gone", api.Exited, 0x7fffffff)
	writeStopTestTerminal(t, runPath, "id-alive", "here", api.Ready, os.Getpid())

	opts := stopOpts{runPath: runPath, all: true, timeout: defaultStopTimeout}
	docs, err := resolveTargets(context.Background(), stopTestLogger(), opts)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 active target, got %d", len(docs))
	}
	if docs[0].Spec.Name != "here" {
		t.Errorf("active target = %q, want 'here'", docs[0].Spec.Name)
	}
}

// Test_StopOneTerminal_KillAfterEscalates covers the graceful→escalate path:
// a child that traps SIGTERM survives the --timeout window, so --kill-after
// escalates to a pgroup SIGKILL. Also exercises the attacher-warning branch.
func Test_StopOneTerminal_KillAfterEscalates(t *testing.T) {
	child := exec.Command("/bin/sh", "-c", `trap '' TERM; sleep 60 & wait`)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	childDone := startBackgroundProc(t, child)
	time.Sleep(100 * time.Millisecond) // let the trap install

	doc := &api.TerminalDoc{
		Spec: api.TerminalSpec{Name: "trap-term"},
		Status: api.TerminalStatus{
			State:     api.Ready,
			Pid:       child.Process.Pid,
			ChildPgid: child.Process.Pid, // Setsid → pgid == pid
			Attachers: []string{"attacher-1"},
		},
	}
	opts := stopOpts{killAfter: true, timeout: 200 * time.Millisecond}
	var out, errOut bytes.Buffer
	res := stopOneTerminal(context.Background(), stopTestLogger(), &out, &errOut, doc, opts)
	if res != resultStopped {
		t.Fatalf("expected resultStopped, got %v\nstdout=%q\nstderr=%q", res, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "attacher") {
		t.Errorf("expected attacher warning on stderr, got %q", errOut.String())
	}
	if !strings.Contains(out.String(), "escalating to SIGKILL") {
		t.Errorf("expected escalation notice, got %q", out.String())
	}
	select {
	case <-childDone:
	case <-time.After(sigkillGrace):
		t.Fatal("child pgroup still alive after escalation")
	}
}

func Test_SendSignal_InvalidPid(t *testing.T) {
	err := sendSignal(0, 0, syscall.SIGTERM)
	if !errors.Is(err, errdefs.ErrSignalProcess) {
		t.Fatalf("expected ErrSignalProcess for pid 0, got: %v", err)
	}
}

func Test_SendSignal_GoneProcessIsNoOp(t *testing.T) {
	// A high pid that doesn't exist, with a non-zero token: pidutil.Match
	// surfaces os.ErrNotExist, which sendSignal treats as "already gone".
	if err := sendSignal(0x7fffffff, 12345, syscall.SIGTERM); err != nil {
		t.Fatalf("expected nil for vanished process, got: %v", err)
	}
}

func Test_WaitForExit_TimeoutAndCancel(t *testing.T) {
	// Self never exits, so the deadline elapses and waitForExit reports false.
	if waitForExit(context.Background(), os.Getpid(), 0, 50*time.Millisecond) {
		t.Error("expected waitForExit to time out on a live process")
	}

	// Cancelled context takes the ctx.Done branch.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitForExit(ctx, os.Getpid(), 0, time.Hour) {
		t.Error("expected waitForExit to report still-alive on cancel")
	}

	// Invalid pid is treated as already exited.
	if !waitForExit(context.Background(), 0, 0, time.Second) {
		t.Error("expected waitForExit(0) to report exited")
	}
}
