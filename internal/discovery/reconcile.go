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

package discovery

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/pidutil"
	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

// isProcessAlive reports whether a Unix process with the given pid is running.
// A pid of 0 or negative is treated as unknown (returns true to avoid false
// positives flipping metadata when the pid was never written).
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return true
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// ESRCH: no such process — definitively gone.
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	// EPERM: process exists but we lack permission to signal it (e.g. owned
	// by another user). Treat as alive rather than invent an Exited state.
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

// IsInstanceAlive reports whether the process instance identified by
// (pid, pidStart) is still running, using the same liveness+PID-reuse rules
// reconcile and prune rely on. Exported so the terminal runner can refuse to
// clobber a live owner's metadata before it overwrites the ID-derived path,
// keeping the runner's notion of "live" identical to reconcile's. See #386.
func IsInstanceAlive(pid int, pidStart uint64) bool {
	return isInstanceAlive(pid, pidStart)
}

// isInstanceAlive extends isProcessAlive with PID-reuse rejection: if a
// start-time token was recorded, the live PID must still belong to the same
// process instance that produced it. A zero token degrades to liveness-only,
// matching the contract documented on pidutil.Match for legacy metadata and
// non-Linux platforms.
func isInstanceAlive(pid int, pidStart uint64) bool {
	if !isProcessAlive(pid) {
		return false
	}
	if pidStart == 0 {
		return true
	}
	ok, err := pidutil.Match(pid, pidStart)
	if err != nil {
		// /proc entry vanished between the signal-0 check and the token read —
		// treat as gone so callers persist Exited.
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		// Other read errors are inconclusive; keep prior behavior (treat as
		// alive) rather than invent an Exited state on a transient /proc error.
		return true
	}
	return ok
}

// ReconcileTerminals checks each terminal's recorded PID and, if the process
// is gone but the metadata still claims a live state, rewrites metadata.json
// with state=Exited. Mutates terminals in place so callers see fresh state.
func ReconcileTerminals(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	terminals []api.TerminalDoc,
) {
	for i := range terminals {
		reconcileTerminal(ctx, logger, runPath, &terminals[i])
	}
}

func reconcileTerminal(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	t *api.TerminalDoc,
) {
	if t.Status.State == api.Exited {
		return
	}
	if isInstanceAlive(t.Status.Pid, t.Status.PidStart) {
		return
	}
	logger.InfoContext(ctx, "reconcileTerminal: marking stale terminal as Exited",
		"id", t.Spec.ID,
		"pid", t.Status.Pid,
		"prev_state", t.Status.State.String(),
	)
	t.Status.State = api.Exited

	dir := t.Status.TerminalRunPath
	if dir == "" {
		dir = filepath.Join(runPath, defaults.TerminalsRunPath, string(t.Spec.ID))
	}
	if err := shared.WriteMetadata(ctx, *t, dir); err != nil {
		logger.WarnContext(ctx, "reconcileTerminal: failed to persist Exited state",
			"id", t.Spec.ID,
			"dir", dir,
			"error", err,
		)
	}
}

// ReconcileClients checks each client's recorded PID and, if the process is
// gone but the metadata still claims a live state, rewrites metadata.json
// with state=ClientExited. Mutates clients in place so callers see fresh state.
func ReconcileClients(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	clients []api.ClientDoc,
) {
	for i := range clients {
		reconcileClient(ctx, logger, runPath, &clients[i])
	}
}

func reconcileClient(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	c *api.ClientDoc,
) {
	if c.Status.State == api.ClientExited {
		return
	}
	if isInstanceAlive(c.Status.Pid, c.Status.PidStart) {
		return
	}
	logger.InfoContext(ctx, "reconcileClient: marking stale client as Exited",
		"id", c.Spec.ID,
		"pid", c.Status.Pid,
		"prev_state", c.Status.State.String(),
	)
	c.Status.State = api.ClientExited

	dir := c.Status.ClientRunPath
	if dir == "" {
		dir = filepath.Join(runPath, defaults.ClientsRunPath, string(c.Spec.ID))
	}
	if err := shared.WriteMetadata(ctx, *c, dir); err != nil {
		logger.WarnContext(ctx, "reconcileClient: failed to persist ClientExited state",
			"id", c.Spec.ID,
			"dir", dir,
			"error", err,
		)
	}
}
