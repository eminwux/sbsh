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

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/shared"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *Exec) CreateMetadata() error {
	sr.logger.Debug("creating metadata", "runPath", sr.getSupervisorDir())
	if err := os.MkdirAll(sr.getSupervisorDir(), 0o700); err != nil {
		sr.logger.Error("failed to create supervisor dir", "dir", sr.getSupervisorDir(), "error", err)
		return fmt.Errorf("mkdir terminal dir: %w", err)
	}

	sr.metadata.Status.State = api.SupervisorInitializing
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
	return filepath.Join(sr.metadata.Spec.RunPath, defaults.SupervisorsRunPath, string(sr.id))
}

func (sr *Exec) updateMetadata() error {
	return shared.WriteMetadata(sr.ctx, sr.metadata, sr.getSupervisorDir())
}

func (sr *Exec) getTerminalMetadata() (*api.TerminalMetadata, error) {
	if sr.terminalClient == nil {
		return nil, errors.New("getTerminalMetadata: terminal client is nil")
	}

	var metadata api.TerminalMetadata
	if err := sr.terminalClient.Metadata(sr.ctx, &metadata); err != nil {
		sr.logger.ErrorContext(sr.ctx, "getTerminalMetadata: failed to get metadata", "error", err)
		return nil, fmt.Errorf("get metadata RPC failed: %w", err)
	}
	sr.logger.InfoContext(sr.ctx, "getTerminalMetadata: metadata retrieved", "metadata", metadata)
	return &metadata, nil
}

func (sr *Exec) getTerminalState() (*api.TerminalStatusMode, error) {
	if sr.terminalClient == nil {
		return nil, errors.New("getTerminalState: terminal client is nil")
	}

	var state api.TerminalStatusMode
	if err := sr.terminalClient.State(sr.ctx, &state); err != nil {
		sr.logger.ErrorContext(sr.ctx, "getTerminalState: failed to get state", "error", err)
		return nil, fmt.Errorf("get state RPC failed: %w", err)
	}
	sr.logger.InfoContext(sr.ctx, "getTerminalState: state retrieved", "state", state)
	return &state, nil
}
