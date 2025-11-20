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

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *Exec) CreateMetadata() error {
	sr.metadataMu.Lock()
	runPath := sr.metadata.Spec.RunPath
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, string(sr.id))
	sr.metadataMu.Unlock()

	sr.logger.Debug("CreateMetadata: creating terminal directory", "dir", dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		sr.logger.Error("CreateMetadata: failed to create terminal dir", "dir", dir, "error", err)
		return fmt.Errorf("mkdir terminal dir: %w", err)
	}

	sr.metadataMu.Lock()
	sr.metadata.Status.Pid = os.Getpid()
	sr.metadata.Status.BaseRunPath = runPath
	sr.metadata.Status.TerminalRunPath = dir
	sr.metadata.Status.LogFile = sr.metadata.Spec.LogFile
	sr.metadata.Status.LogLevel = sr.metadata.Spec.LogLevel
	sr.metadata.Status.CaptureFile = sr.metadata.Spec.CaptureFile
	sr.metadata.Status.State = api.Initializing

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
	return filepath.Join(sr.metadata.Spec.RunPath, defaults.TerminalsRunPath, string(sr.id))
}

func (sr *Exec) updateMetadata() error {
	sr.metadataMu.RLock()
	metadataCopy := sr.metadata
	dir := filepath.Join(sr.metadata.Spec.RunPath, defaults.TerminalsRunPath, string(sr.id))
	sr.metadataMu.RUnlock()
	return shared.WriteMetadata(sr.ctx, metadataCopy, dir)
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
	sr.metadata.Status.Attachers = strIDs
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
