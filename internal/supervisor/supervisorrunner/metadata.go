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

package supervisorrunner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *Exec) CreateMetadata() error {
	sr.logger.Debug("creating metadata", "runPath", sr.getSupervisorDir())
	if err := os.MkdirAll(sr.getSupervisorDir(), 0o700); err != nil {
		sr.logger.Error("failed to create supervisor dir", "dir", sr.getSupervisorDir(), "error", err)
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	sr.metadata.Status.Pid = os.Getpid()
	sr.metadata.Status.BaseRunPath = sr.metadata.Spec.RunPath
	sr.metadata.Status.SupervisorRunPath = sr.getSupervisorDir()

	sr.logger.Info("metadata values set",
		"Spec", sr.metadata.Spec,
		"Status", sr.metadata.Status,
	)

	err := sr.updateMetadata()
	if err != nil {
		sr.logger.Error("failed to update metadata", "error", err)
		return err
	}
	sr.logger.Info("metadata created successfully")
	return nil
}

func (sr *Exec) getSupervisorDir() string {
	return filepath.Join(sr.metadata.Spec.RunPath, "supervisors", string(sr.id))
}

func (sr *Exec) updateMetadata() error {
	return shared.WriteMetadata(sr.ctx, sr.metadata, sr.getSupervisorDir())
}

func (sr *Exec) getSessionMetadata() (*api.SessionMetadata, error) {
	if sr.sessionClient == nil {
		return nil, errors.New("getSessionMetadata: session client is nil")
	}

	var metadata api.SessionMetadata
	if err := sr.sessionClient.Metadata(sr.ctx, &metadata); err != nil {
		sr.logger.ErrorContext(sr.ctx, "getSessionMetadata: failed to get metadata", "error", err)
		return nil, fmt.Errorf("get metadata RPC failed: %w", err)
	}
	sr.logger.InfoContext(sr.ctx, "getSessionMetadata: metadata retrieved", "metadata", metadata)
	return &metadata, nil
}
