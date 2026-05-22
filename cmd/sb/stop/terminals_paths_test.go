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
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/pidutil"
	"github.com/eminwux/sbsh/pkg/api"
)

// stopRPCService is a minimal net/rpc service exposing only TerminalController.Stop.
// The command layer (cmd/sb/stop) talks to the controller over a Unix socket via
// pkg/rpcclient/terminal, so the graceful path cannot be driven by the in-process
// ControllerTest fake the issue referenced; it needs a real jsonrpc listener.
type stopRPCService struct {
	onStop func(args *api.StopArgs) error
}

func (s *stopRPCService) Stop(args *api.StopArgs, _ *api.Empty) error {
	return s.onStop(args)
}

// startStopRPCServer stands up a jsonrpc Unix server at sockPath that registers a
// TerminalController.Stop handler. The server is torn down via t.Cleanup.
func startStopRPCServer(t *testing.T, sockPath string, onStop func(*api.StopArgs) error) {
	t.Helper()
	srv := rpc.NewServer()
	if err := srv.RegisterName(api.TerminalService, &stopRPCService{onStop: onStop}); err != nil {
		t.Fatalf("RegisterName: %v", err)
	}
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	go func() {
		for {
			conn, acceptErr := l.Accept()
			if acceptErr != nil {
				return
			}
			go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
}

// spawnSessionProc starts /bin/sh -c script in its own session (Setsid → pgid ==
// pid) so the test can reap the whole pgroup, records a goroutine that Wait()s to
// avoid a zombie, and returns the pid, its start-time token, and a channel that
// closes once the process is reaped.
func spawnSessionProc(t *testing.T, script string) (pid int, pidStart uint64, done <-chan struct{}) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	d := startBackgroundProc(t, cmd)
	pid = cmd.Process.Pid
	var err error
	pidStart, err = pidutil.StartTime(pid)
	if err != nil {
		t.Fatalf("StartTime(%d): %v", pid, err)
	}
	return pid, pidStart, d
}

// Test_StopOneTerminal_RPCGraceful covers the graceful path: the controller
// answers the Stop RPC, so no fallback signal is sent. The recorded process
// ignores SIGTERM, so it can only be taken down by the RPC handler's SIGKILL —
// a resultStopped therefore proves the graceful path ran (a SIGTERM fallback
// would leave the trap-TERM process alive and yield resultFailed).
func Test_StopOneTerminal_RPCGraceful(t *testing.T) {
	pid, pidStart, done := spawnSessionProc(t, `trap "" TERM; sleep 60`)

	var calls atomic.Int32
	sockPath := t.TempDir() + "/controller.sock"
	startStopRPCServer(t, sockPath, func(_ *api.StopArgs) error {
		calls.Add(1)
		// Emulate the controller shutting its process down on Stop.
		_ = syscall.Kill(pid, syscall.SIGKILL)
		return nil
	})

	doc := &api.TerminalDoc{
		Spec: api.TerminalSpec{Name: "graceful"},
		Status: api.TerminalStatus{
			State:      api.Ready,
			Pid:        pid,
			PidStart:   pidStart,
			SocketFile: sockPath,
		},
	}
	opts := stopOpts{timeout: 5 * time.Second}
	var stdout, stderr bytes.Buffer

	res := stopOneTerminal(context.Background(), stopTestLogger(), &stdout, &stderr, doc, opts)
	if res != resultStopped {
		t.Fatalf("expected resultStopped, got %v\nstdout=%q stderr=%q", res, stdout.String(), stderr.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("expected exactly one Stop RPC call, got %d", calls.Load())
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process still alive after graceful stop")
	}
}

// Test_StopOneTerminal_PidReuseRejected covers PID-reuse rejection: the recorded
// PID is alive but its PidStart token does not match the live process, so
// processIsOurs reports false and stopOneTerminal treats the terminal as already
// exited without sending any signal. The process must survive the call.
func Test_StopOneTerminal_PidReuseRejected(t *testing.T) {
	pid, pidStart, _ := spawnSessionProc(t, `sleep 60`)

	doc := &api.TerminalDoc{
		Spec: api.TerminalSpec{Name: "reused"},
		Status: api.TerminalStatus{
			State:    api.Ready,
			Pid:      pid,
			PidStart: pidStart + 1, // deliberately mismatched token
		},
	}
	opts := stopOpts{timeout: 5 * time.Second}
	var stdout, stderr bytes.Buffer

	res := stopOneTerminal(context.Background(), stopTestLogger(), &stdout, &stderr, doc, opts)
	if res != resultAlreadyExited {
		t.Fatalf("expected resultAlreadyExited, got %v\nstdout=%q stderr=%q", res, stdout.String(), stderr.String())
	}
	if !processAlive(pid) {
		t.Fatal("process was signalled despite PidStart mismatch; expected it left untouched")
	}
}

// Test_RunStopTerminals_AllAggregation covers the --all aggregation over multiple
// targets: one terminal that stops (SIGTERM fallback) and one that fails to stop
// (traps SIGTERM, no --kill-after, short timeout). A dead-PID terminal is also on
// disk to confirm the reconcile filter excludes it from the target set entirely
// (it counts as neither stopped nor failed). The aggregated summary counts must
// reflect the live mix and the per-terminal failure must surface as ErrStopTerminal.
func Test_RunStopTerminals_AllAggregation(t *testing.T) {
	runPath := t.TempDir()

	// Stoppable: plain sleep, dies on the SIGTERM fallback.
	stopPid, _, stopDone := spawnSessionProc(t, `sleep 60`)
	writeStopTestTerminal(t, runPath, "id-stop", "stoppable", api.Ready, stopPid)

	// Dead PID: reconciled to Exited and filtered out before aggregation, so it
	// never reaches stopOneTerminal (the alreadyExited count via --all is always 0).
	writeStopTestTerminal(t, runPath, "id-dead", "dead", api.Ready, 0x7fffffff)

	// Failing: traps SIGTERM and --kill-after is off, so it never exits within
	// the short timeout → resultFailed.
	failPid, _, _ := spawnSessionProc(t, `trap "" TERM; sleep 60`)
	writeStopTestTerminal(t, runPath, "id-fail", "stubborn", api.Ready, failPid)

	opts := stopOpts{
		runPath: runPath,
		all:     true,
		timeout: 300 * time.Millisecond,
	}
	var stdout, stderr bytes.Buffer
	err := runStopTerminals(context.Background(), stopTestLogger(), &stdout, &stderr, opts)
	if err == nil {
		t.Fatal("expected aggregated error from the failing terminal, got nil")
	}
	if !errors.Is(err, errdefs.ErrStopTerminal) {
		t.Fatalf("expected ErrStopTerminal, got %v", err)
	}

	// Dead terminal is filtered by reconcile, so the live mix is 1 stopped + 1 failed.
	out := stdout.Bytes()
	for _, want := range []string{"stopped 1 terminal(s)", "0 already exited", "1 error(s)"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("aggregation summary missing %q\ngot: %q", want, stdout.String())
		}
	}

	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("stoppable terminal still alive after --all stop")
	}
}
