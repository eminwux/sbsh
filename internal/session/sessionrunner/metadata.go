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

package sessionrunner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *Exec) CreateMetadata() error {
	sr.logger.Debug("CreateMetadata: creating session directory", "dir", sr.getSessionDir())
	if err := os.MkdirAll(sr.getSessionDir(), 0o700); err != nil {
		sr.logger.Error("CreateMetadata: failed to create session dir", "dir", sr.getSessionDir(), "error", err)
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	sr.metadata.Status.Pid = os.Getpid()
	sr.metadata.Status.BaseRunPath = sr.metadata.Spec.RunPath
	sr.metadata.Status.SessionRunPath = sr.getSessionDir()
	sr.metadata.Status.LogFile = sr.metadata.Spec.LogFile
	sr.metadata.Status.LogLevel = sr.metadata.Spec.LogLevel
	sr.metadata.Status.CaptureFile = sr.metadata.Spec.CaptureFile

	sr.logger.Info("CreateMetadata: session metadata set", "Spec", sr.metadata.Spec, "Status", sr.metadata.Status)

	err := sr.updateMetadata()
	if err != nil {
		sr.logger.Error("CreateMetadata: failed to update metadata", "error", err)
		return err
	}
	sr.logger.Info("CreateMetadata: metadata created successfully")
	return nil
}

func (sr *Exec) getSessionDir() string {
	return filepath.Join(sr.metadata.Spec.RunPath, "sessions", string(sr.id))
}

func (sr *Exec) updateMetadata() error {
	return shared.WriteMetadata(sr.ctx, sr.metadata, sr.getSessionDir())
}

func (sr *Exec) updateSessionState(status api.SessionStatusMode) error {
	sr.metadata.Status.State = status

	if err := sr.updateMetadata(); err != nil {
		sr.logger.Warn("failed to update metadata on close", "id", sr.id, "err", err)
		return err
	}
	return nil
}

func (sr *Exec) updateSessionAttachers() error {
	clientList := sr.getClientList()
	var strIDs []string
	for _, idPtr := range clientList {
		if idPtr != nil {
			strIDs = append(strIDs, string(*idPtr))
		}
	}
	sr.metadata.Status.Attachers = strIDs

	if err := sr.updateMetadata(); err != nil {
		sr.logger.Warn("failed to update metadata on close", "id", sr.id, "err", err)
		return err
	}
	return nil
}

func (sr *Exec) Metadata() (*api.SessionMetadata, error) {
	return &sr.metadata, nil
}
