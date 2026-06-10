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
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/pidutil"
	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

// metadataWriteDeniedMsg is the single actionable warning logged (once per
// runner) when a metadata write is denied because the terminal dir is not
// writable by the closing uid. It names the ownership contract embedders must
// satisfy so the message is self-diagnosing rather than a bare errno.
//
// Ownership/mode contract for RunPath/terminals/<id> (the dir that holds
// metadata.json): sbsh writes metadata.json via an atomic
// create-temp-then-rename, which needs write+exec on the directory at every
// point in the terminal's lifecycle — not just at create. The legacy layout
// creates the dir 0o700, owned by the creating uid. When an embedder runs
// create and close under different uids (e.g. setup as root, the in-cell
// process as a non-root container uid), the closing uid cannot create the
// temp file and the write is denied. Embedders that span uids must either set
// TerminalSpec.MetadataDir to a directory they pre-own and keep writable by
// every uid in the terminal's lifecycle (e.g. group-writable under a shared
// gid), or accept that close-time metadata updates will not persist — in which
// case this single warning, not repeated spam, is the surfaced signal.
const metadataWriteDeniedMsg = "metadata write denied: terminal dir is not writable by the closing uid; " +
	"the embedder must keep RunPath/terminals/<id> (or TerminalSpec.MetadataDir) writable by every uid " +
	"that runs the terminal lifecycle (create and close) — see TerminalSpec docs"

func (sr *Exec) CreateMetadata() error {
	sr.metadataMu.Lock()
	runPath := sr.metadata.Spec.RunPath
	metadataDirOverride := sr.metadata.Spec.MetadataDir
	sr.metadataMu.Unlock()

	dir, embedderOwnsDir := resolveMetadataDir(runPath, metadataDirOverride, sr.id)

	// Refuse to overwrite metadata.json at a path whose current owner is still
	// alive. CreateMetadata writes the ID-derived path unconditionally; without
	// this guard a second start reusing a live terminal's --id (which a
	// different --name sails past the CLI name check) clobbers the live
	// terminal's pid/name/state, making it invisible and letting prune
	// os.RemoveAll a still-running terminal's run dir. The socket probe in
	// OpenSocketCtrl runs only after this write, so detection has to happen
	// here, before the overwrite. See #386.
	if owner := liveMetadataOwner(dir); owner != nil {
		sr.logger.Error(
			"CreateMetadata: refusing to overwrite metadata owned by a live terminal",
			"dir", dir,
			"owner_id", owner.Spec.ID,
			"owner_name", owner.Spec.Name,
			"owner_pid", owner.Status.Pid,
		)
		return fmt.Errorf("%w: %q (pid %d)", errdefs.ErrTerminalIDInUse, owner.Spec.ID, owner.Status.Pid)
	}

	if embedderOwnsDir {
		// MetadataDir is set: the embedder pre-allocated this directory
		// (e.g. via a bind mount) and owns its contents. Do not MkdirAll —
		// avoid silently creating a parent the embedder didn't intend, and
		// avoid puncturing the embedder's "this dir is fully mine" contract
		// with sbsh-injected subdirs. See pkg/api.TerminalSpec.MetadataDir.
		sr.logger.Debug(
			"CreateMetadata: using embedder-owned metadata directory",
			"dir", dir,
		)
	} else {
		sr.logger.Debug("CreateMetadata: creating terminal directory", "dir", dir)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			sr.logger.Error("CreateMetadata: failed to create terminal dir", "dir", dir, "error", err)
			return fmt.Errorf("mkdir terminal dir: %w", err)
		}
	}

	pid := os.Getpid()
	pidStart, startErr := pidutil.StartTime(pid)
	if startErr != nil {
		sr.logger.Warn("CreateMetadata: failed to capture pid start-time token", "pid", pid, "error", startErr)
	}

	sr.metadataMu.Lock()
	sr.metadata.Status.Pid = pid
	sr.metadata.Status.PidStart = pidStart
	sr.metadata.Status.BaseRunPath = runPath
	sr.metadata.Status.TerminalRunPath = dir
	sr.metadata.Status.LogFile = sr.metadata.Spec.LogFile
	sr.metadata.Status.LogLevel = sr.metadata.Spec.LogLevel
	sr.metadata.Status.CaptureFile = sr.metadata.Spec.CaptureFile
	sr.metadata.Status.State = api.Initializing
	if sr.metadata.Metadata.CreatedAt.IsZero() {
		sr.metadata.Metadata.CreatedAt = time.Now().UTC()
	}

	sr.logger.Info("CreateMetadata: terminal metadata set", "Spec", sr.metadata.Spec, "Status", sr.metadata.Status)
	sr.metadataMu.Unlock()

	err := sr.updateMetadata()
	if err != nil {
		sr.logger.Error("CreateMetadata: failed to update metadata", "error", err)
		return err
	}
	sr.logger.Info("CreateMetadata: metadata created successfully")
	return nil
}

// liveMetadataOwner reads an existing metadata.json under dir and returns the
// decoded doc when it represents an active owner — recorded state is not Exited
// and the owner process instance is still alive — or nil when no metadata
// exists, it is unreadable/corrupt, the prior owner already exited, or its
// process is gone. This is the reconcile/prune notion of "active": a terminal
// is live iff State != Exited and IsInstanceAlive(pid, pidStart), so a recycled
// ID whose prior owner exited cleanly (State==Exited) or died (pid gone) is
// correctly treated as free for reuse — matching the name-recycling semantics
// of VerifyTerminalNameAvailable. CreateMetadata is called once per runner
// lifecycle before the runner writes any metadata of its own, so a fresh start
// never matches itself here.
func liveMetadataOwner(dir string) *api.TerminalDoc {
	if dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		// ENOENT (fresh ID) or any read error: no live owner to protect.
		return nil
	}
	var doc api.TerminalDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		// Corrupt metadata cannot identify a live owner; do not block the start.
		return nil
	}
	if doc.Status.State == api.Exited {
		// Prior owner exited cleanly; the ID is free to recycle.
		return nil
	}
	if !discovery.IsInstanceAlive(doc.Status.Pid, doc.Status.PidStart) {
		return nil
	}
	return &doc
}

func (sr *Exec) getTerminalDir() string {
	sr.metadataMu.RLock()
	defer sr.metadataMu.RUnlock()
	dir, _ := resolveMetadataDir(sr.metadata.Spec.RunPath, sr.metadata.Spec.MetadataDir, sr.id)
	return dir
}

func (sr *Exec) updateMetadata() error {
	sr.metadataMu.RLock()
	metadataCopy := sr.metadata
	dir, _ := resolveMetadataDir(sr.metadata.Spec.RunPath, sr.metadata.Spec.MetadataDir, sr.id)
	sr.metadataMu.RUnlock()
	return shared.WriteMetadata(sr.ctx, metadataCopy, dir)
}

// resolveMetadataDir picks the directory where metadata.json is written.
// When metadataDirOverride is non-empty the embedder owns that directory
// (e.g. kukeon bind-mounts /run/kukeon/tty into the container) and the
// runner places metadata.json there without creating any sbsh-injected
// subdir; the second return is true on that branch so callers can skip
// MkdirAll. Empty preserves the legacy runPath/terminals/<id>/ layout
// scanned by pkg/discovery.ScanTerminals.
func resolveMetadataDir(runPath, metadataDirOverride string, id api.ID) (string, bool) {
	if metadataDirOverride != "" {
		return metadataDirOverride, true
	}
	return filepath.Join(runPath, defaults.TerminalsRunPath, string(id)), false
}

// noteMetadataWriteErr logs a metadata-write failure with permission-denied
// spam suppression. A permission error (the terminal dir is not writable by
// the closing uid — typically an embedded runtime where create and close run
// under different uids) is logged once at WARN with the actionable
// ownership-contract message; subsequent permission failures drop to Debug so
// the close and client-cleanup hooks do not re-spam the same condition.
// Non-permission errors always log at WARN. See #345.
func (sr *Exec) noteMetadataWriteErr(where string, err error) {
	if errors.Is(err, fs.ErrPermission) {
		first := false
		sr.metadataWriteDeniedOnce.Do(func() { first = true })
		if first {
			sr.logger.Warn(metadataWriteDeniedMsg, "id", sr.id, "dir", sr.getTerminalDir(), "where", where, "err", err)
		} else {
			sr.logger.Debug("metadata write still denied", "id", sr.id, "where", where, "err", err)
		}
		return
	}
	sr.logger.Warn("failed to update metadata on close", "id", sr.id, "where", where, "err", err)
}

// updateTerminalState advances the persisted Status.State by writing the new
// state to disk first and committing it in-memory only on a successful
// persist. The in-memory commit is gated on the write so a transient
// persistence failure (ENOSPC, perm-denied — #345, #373) cannot strand
// Status.State at an intermediate value (e.g. PostAttach) that no future
// caller can advance past — `PostAttachShell`'s `for State != Ready` spin
// would then deadlock every subsequent attach until the runner is
// recreated. See #373.
func (sr *Exec) updateTerminalState(status api.TerminalStatusMode) error {
	sr.metadataMu.RLock()
	metadataCopy := sr.metadata
	dir, _ := resolveMetadataDir(sr.metadata.Spec.RunPath, sr.metadata.Spec.MetadataDir, sr.id)
	sr.metadataMu.RUnlock()
	metadataCopy.Status.State = status

	if err := shared.WriteMetadata(sr.ctx, metadataCopy, dir); err != nil {
		sr.noteMetadataWriteErr("updateTerminalState", err)
		return err
	}

	sr.metadataMu.Lock()
	sr.metadata.Status.State = status
	sr.metadataMu.Unlock()
	return nil
}

func (sr *Exec) updateTerminalAttachers() error {
	clientList := sr.getClientList()
	var strIDs []string
	for _, idPtr := range clientList {
		if idPtr != nil {
			strIDs = append(strIDs, string(*idPtr))
		}
	}
	sr.metadataMu.Lock()
	previous := len(sr.metadata.Status.Attachers)
	sr.metadata.Status.Attachers = strIDs
	if len(strIDs) > previous {
		sr.metadata.Status.LastAttachedAt = time.Now().UTC()
	}
	sr.metadataMu.Unlock()

	if err := sr.updateMetadata(); err != nil {
		sr.noteMetadataWriteErr("updateTerminalAttachers", err)
		return err
	}
	return nil
}

func (sr *Exec) Metadata() (*api.TerminalDoc, error) {
	sr.metadataMu.RLock()
	defer sr.metadataMu.RUnlock()
	// Return a copy to avoid race conditions
	metadataCopy := sr.metadata
	return &metadataCopy, nil
}
