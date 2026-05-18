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
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
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
