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
	"os"
	"path/filepath"
	"sbsh/pkg/common"
)

func (sr *SupervisorRunnerExec) CreateMetadata() error {

	if err := os.MkdirAll(sr.getSupervisorsDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	return common.WriteMetadata(sr.ctx, sr.spec, sr.getSupervisorsDir())
}

func (sr *SupervisorRunnerExec) getSupervisorsDir() string {
	return filepath.Join(sr.runPath, string(sr.id))
}
