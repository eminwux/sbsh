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
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/internal/common"
)

func (sr *SessionRunnerExec) CreateMetadata() error {
	if err := os.MkdirAll(sr.getSessionDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	sr.metadata.Status.Pid = os.Getpid()
	sr.metadata.Status.BaseRunPath = sr.metadata.Spec.RunPath
	sr.metadata.Status.SessionRunPath = sr.getSessionDir()
	sr.metadata.Status.LogFile = sr.metadata.Spec.LogFile
	sr.metadata.Status.LogLevel = sr.metadata.Spec.LogLevel
	sr.metadata.Status.CaptureFile = sr.metadata.Spec.CaptureFile

	// Log metadata values with slog.Debug
	slog.Debug("Session metadata",
		"Spec", sr.metadata.Spec,
		"Status", sr.metadata.Status,
	)

	return sr.updateMetadata()
}

func (sr *SessionRunnerExec) getSessionDir() string {
	return filepath.Join(sr.metadata.Spec.RunPath, "sessions", string(sr.id))
}

func (sr *SessionRunnerExec) updateMetadata() error {
	return common.WriteMetadata(sr.ctx, sr.metadata, sr.getSessionDir())
}
