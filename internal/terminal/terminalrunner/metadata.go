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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/pidutil"
	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *Exec) CreateMetadata() error {
	sr.metadataMu.Lock()
	runPath := sr.metadata.Spec.RunPath
	metadataDirOverride := sr.metadata.Spec.MetadataDir
	sr.metadataMu.Unlock()

	dir, embedderOwnsDir := resolveMetadataDir(runPath, metadataDirOverride, sr.id)

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

func (sr *Exec) updateTerminalState(status api.TerminalStatusMode) error {
	sr.metadataMu.Lock()
	sr.metadata.Status.State = status
	sr.metadataMu.Unlock()

	if err := sr.updateMetadata(); err != nil {
		sr.logger.Warn("failed to update metadata on close", "id", sr.id, "err", err)
		return err
	}
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
		sr.logger.Warn("failed to update metadata on close", "id", sr.id, "err", err)
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
