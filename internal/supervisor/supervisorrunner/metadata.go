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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/internal/common"
)

func (sr *SupervisorRunnerExec) CreateMetadata() error {
	slog.Debug("[supervisor] creating metadata", "runPath", sr.getSupervisorDir())
	if err := os.MkdirAll(sr.getSupervisorDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	sr.metadata.Status.Pid = os.Getpid()
	sr.metadata.Status.BaseRunPath = sr.metadata.Spec.RunPath
	sr.metadata.Status.SupervisorRunPath = sr.getSupervisorDir()

	slog.Debug("[supervisor] metadata values",
		"Spec", sr.metadata.Spec,
		"Status", sr.metadata.Status,
	)

	return sr.updateMetadata()
}

func (sr *SupervisorRunnerExec) getSupervisorDir() string {
	return filepath.Join(sr.metadata.Spec.RunPath, "supervisors", string(sr.id))
}

func (sr *SupervisorRunnerExec) updateMetadata() error {
	return common.WriteMetadata(sr.ctx, sr.metadata, sr.getSupervisorDir())
}
