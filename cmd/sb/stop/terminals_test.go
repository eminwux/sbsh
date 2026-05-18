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
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/pidutil"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func Test_ErrLoggerNotFound_Terminal_RunE(t *testing.T) {
	cmd := NewStopTerminalsCmd()
	ctx := context.Background()
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{"foo"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ResolveStopOpts(t *testing.T) {
	type tc struct {
		name    string
		args    []string
		setup   func()
		wantErr error
	}
	cases := []tc{
		{
			name:    "no name and not --all errors",
			args:    []string{},
			setup:   func() {},
			wantErr: errdefs.ErrNoTerminalIdentifier,
		},
		{
			name: "name with --all errors",
			args: []string{"foo"},
			setup: func() {
				viper.Set(config.SB_STOP_ALL.ViperKey, true)
			},
			wantErr: errdefs.ErrInvalidFlag,
		},
		{
			name: "zero timeout errors",
			args: []string{"foo"},
			setup: func() {
				viper.Set(config.SB_STOP_TIMEOUT.ViperKey, 0)
			},
			wantErr: errdefs.ErrInvalidFlag,
		},
		{
			name: "name only succeeds",
			args: []string{"foo"},
			setup: func() {
				viper.Set(config.SB_STOP_TIMEOUT.ViperKey, "5s")
			},
			wantErr: nil,
		},
		{
			name: "--all with no name succeeds",
			args: []string{},
			setup: func() {
				viper.Set(config.SB_STOP_ALL.ViperKey, true)
				viper.Set(config.SB_STOP_TIMEOUT.ViperKey, "5s")
			},
			wantErr: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			viper.Reset()
			c.setup()
			_ = NewStopTerminalsCmd() // build flag bindings into viper
			_, err := resolveStopOpts(c.args)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("expected %v, got %v", c.wantErr, err)
			}
		})
	}
}

func Test_ProcessAlive_InvalidPid(t *testing.T) {
	if processAlive(0) {
		t.Fatal("expected pid 0 to be reported dead")
	}
	if processAlive(-1) {
		t.Fatal("expected pid -1 to be reported dead")
	}
}

func Test_ProcessIsOurs_DifferentToken(t *testing.T) {
	pid := os.Getpid()
	// any non-zero token that won't match self's actual start time forces
	// pidutil.Match to return false, proving processIsOurs rejects mismatches
	// even when the pid is alive.
	if processIsOurs(pid, 1) {
		t.Fatal("expected processIsOurs to reject a live pid with a mismatching pidStart token")
	}
}

func Test_ProcessIsOurs_ZeroTokenFallsBack(t *testing.T) {
	// Zero pidStart means metadata predates the token; fall back to liveness.
	if !processIsOurs(os.Getpid(), 0) {
		t.Fatal("expected processIsOurs(self, 0) to be true (liveness fallback)")
	}
	if processIsOurs(0, 0) {
		t.Fatal("expected processIsOurs(0, 0) to be false")
	}
}

func writeStopTestTerminal(t *testing.T, runPath, id, name string, state api.TerminalStatusMode, pid int) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Spec:       api.TerminalSpec{ID: api.ID(id), Name: name, RunPath: runPath},
		Status: api.TerminalStatus{
			State:           state,
			Pid:             pid,
			TerminalRunPath: dir,
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

func readStopTestTerminalState(t *testing.T, runPath, id string) api.TerminalStatusMode {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(runPath, defaults.TerminalsRunPath, id, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var doc api.TerminalDoc
	if unmarshalErr := json.Unmarshal(b, &doc); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	return doc.Status.State
}

// Test_ResolveTargets_SingleName_PersistsExited covers the previously-missing
// reconcile step on the single-target name path: a stale terminal whose
// recorded PID is dead should be persisted as Exited after resolveTargets
// runs, matching what the --all branch already does.
func Test_ResolveTargets_SingleName_PersistsExited(t *testing.T) {
	const deadPid = 0x7fffffff // very-high PID unlikely to exist
	runPath := t.TempDir()
	writeStopTestTerminal(t, runPath, "id-dead", "alpha", api.Ready, deadPid)

	opts := stopOpts{
		runPath: runPath,
		name:    "alpha",
		timeout: defaultStopTimeout,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	docs, err := resolveTargets(context.Background(), logger, opts)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Status.State != api.Exited {
		t.Fatalf("in-memory: expected Exited after reconcile, got %v", docs[0].Status.State)
	}
	if got := readStopTestTerminalState(t, runPath, "id-dead"); got != api.Exited {
		t.Fatalf("persisted: expected Exited after reconcile, got %v", got)
	}
}

func Test_StopViaRPC_StaleSocket_RespectsTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "controller.sock")

	// Fake a stale controller socket: bind a Unix listener, prevent the
	// default unlink-on-close so the socket file survives, then close the
	// listener. Subsequent dial() returns ECONNREFUSED quickly; without a
	// bounded ctx the retry loop burns ~600ms of inter-attempt delays
	// (and up to ~9.6s if dial itself stalls — see #251).
	addr := &net.UnixAddr{Name: sockPath, Net: "unix"}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	l.SetUnlinkOnClose(false)
	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("Close listener: %v", closeErr)
	}
	if _, statErr := os.Stat(sockPath); statErr != nil {
		t.Fatalf("expected stale socket file at %s, stat: %v", sockPath, statErr)
	}

	doc := &api.TerminalDoc{
		Spec:   api.TerminalSpec{Name: "stale"},
		Status: api.TerminalStatus{SocketFile: sockPath},
	}

	timeout := 100 * time.Millisecond
	start := time.Now()
	rpcErr := stopViaRPC(context.Background(), slog.Default(), doc, timeout)
	elapsed := time.Since(start)

	if rpcErr == nil {
		t.Fatal("stopViaRPC against a stale socket: expected error, got nil")
	}
	// Generous slack for CI variance but well below the retry-delay budget
	// (~600ms) and orders of magnitude below the pre-fix 9.6s ceiling.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("stopViaRPC took %v with timeout=%v; expected <500ms (pre-fix: 0.6s-9.6s)", elapsed, timeout)
	}
}

// startBackgroundProc spawns cmd, fires a one-shot goroutine that Wait()s
// on it so the test process reaps the zombie, and registers a Cleanup that
// SIGKILLs the whole pgroup as a safety net. The returned channel closes
// once Wait() returns, letting the test assert "process is gone" without
// racing zombie state.
func startBackgroundProc(t *testing.T, cmd *exec.Cmd) <-chan struct{} {
	t.Helper()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", cmd.Args, err)
	}
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		// Best-effort pgroup kill in case the test fails before our
		// signal lands. ESRCH (group already drained) is fine.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return done
}

// Test_StopOneTerminal_Force_ReapsChildPgroupOnSIGHUPTrap covers #252:
// `sb stop --force` must SIGKILL the recorded child pgroup before SIGKILL'ing
// the controller, otherwise a shell that traps or ignores SIGHUP (the
// PTY-master-close fallback) survives reparented to init.
//
// We approximate the runner's two-process layout without spinning up a real
// controller/PTY: a long-sleeping "controller" stand-in, plus a SIGHUP-and-
// SIGTERM-trapping child shell in its own session (Setsid → pgid == pid).
// Pre-fix, --force only SIGKILLs the controller and the child shell stays
// alive past sigkillGrace; post-fix the pgroup-kill takes it down.
func Test_StopOneTerminal_Force_ReapsChildPgroupOnSIGHUPTrap(t *testing.T) {
	// Child shell: trap HUP and TERM so only SIGKILL takes it (and the
	// backgrounded sleep) down. Setsid puts it in its own pgroup, mirroring
	// the runner's prepareTerminalCommand.
	child := exec.Command("/bin/sh", "-c", `trap '' HUP TERM; sleep 60 & wait`)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	childDone := startBackgroundProc(t, child)

	// Controller stand-in: a plain sleep we can SIGKILL by PID.
	parent := exec.Command("/bin/sh", "-c", `sleep 60`)
	parentDone := startBackgroundProc(t, parent)

	// Give the child shell time to install the trap before we signal —
	// without this, SIGKILL on the pgroup can win the race before `trap`
	// runs, which trivially passes the test for the wrong reason.

	time.Sleep(100 * time.Millisecond)

	parentPidStart, err := pidutil.StartTime(parent.Process.Pid)
	if err != nil {
		t.Fatalf("StartTime(parent=%d): %v", parent.Process.Pid, err)
	}

	doc := &api.TerminalDoc{
		Spec: api.TerminalSpec{Name: "trap-hup"},
		Status: api.TerminalStatus{
			State:     api.Ready,
			Pid:       parent.Process.Pid,
			PidStart:  parentPidStart,
			ChildPgid: child.Process.Pid, // Setsid → pgid == child.Pid
		},
	}
	opts := stopOpts{force: true, timeout: defaultStopTimeout}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var stdout, stderr bytes.Buffer

	start := time.Now()
	res := stopOneTerminal(context.Background(), logger, &stdout, &stderr, doc, opts)
	elapsed := time.Since(start)

	if res != resultStopped {
		t.Fatalf("expected resultStopped, got %v\nstdout=%q\nstderr=%q", res, stdout.String(), stderr.String())
	}
	if elapsed > sigkillGrace {
		t.Fatalf("stopOneTerminal --force took %v; expected <= sigkillGrace (%v)", elapsed, sigkillGrace)
	}

	// Controller is reaped by SIGKILL on doc.Status.Pid.
	select {
	case <-parentDone:
	case <-time.After(sigkillGrace):
		t.Fatalf("controller stand-in still alive after sigkillGrace=%v", sigkillGrace)
	}
	// Load-bearing assertion for #252: the trap-HUP child must be reaped
	// by the pgroup-kill, not survive past sigkillGrace.
	select {
	case <-childDone:
	case <-time.After(sigkillGrace):
		t.Fatalf("child shell pgroup still alive after sigkillGrace=%v; --force only reaped controller", sigkillGrace)
	}
}

// Test_SendPgroupSignal_ZeroPgidIsNoOp guards the critical invariant that
// Kill(0, sig) — which on POSIX means "signal every process in the caller's
// own pgroup" — must never be issued. Older terminals on disk have no
// ChildPgid recorded; the field defaults to zero.
func Test_SendPgroupSignal_ZeroPgidIsNoOp(t *testing.T) {
	if err := sendPgroupKill(0); err != nil {
		t.Fatalf("sendPgroupKill(0) = %v; want nil (no-op)", err)
	}
	if err := sendPgroupKill(-1); err != nil {
		t.Fatalf("sendPgroupKill(-1) = %v; want nil (no-op)", err)
	}
}

// Test_SendPgroupSignal_GoneGroupIsNoOp covers the ESRCH path: a pgid whose
// members have already drained must not surface as an error to the caller.
func Test_SendPgroupSignal_GoneGroupIsNoOp(t *testing.T) {
	probe := exec.Command("/bin/sh", "-c", `exit 0`)
	probe.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := probe.Start(); err != nil {
		t.Fatalf("start probe: %v", err)
	}
	pgid := probe.Process.Pid
	if _, err := probe.Process.Wait(); err != nil {
		t.Fatalf("wait probe: %v", err)
	}
	// Brief settle so the pgroup is fully drained — exit(0) on the
	// leader can race ahead of pgroup teardown on slow CI.

	time.Sleep(50 * time.Millisecond)
	if err := sendPgroupKill(pgid); err != nil {
		t.Fatalf("sendPgroupKill(drained pgid=%d) = %v; want nil (ESRCH suppressed)", pgid, err)
	}
}
