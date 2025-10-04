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
	"net"
	"os"
	"path/filepath"
)

func (sr *SupervisorRunnerExec) OpenSocketCtrl() error {

	sr.supervisorSocketCtrl = filepath.Join(sr.getSupervisorsDir(), "ctrl.sock")
	slog.Debug(fmt.Sprintf("[supervisor] CTRL socket: %s", sr.supervisorSocketCtrl))

	// remove stale socket if it exists
	if _, err := os.Stat(sr.supervisorSocketCtrl); err == nil {
		_ = os.Remove(sr.supervisorSocketCtrl)
	}
	ln, err := net.Listen("unix", sr.supervisorSocketCtrl)
	if err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] cannot listen: %v", err))
		return fmt.Errorf("cannot listen: %v", err)
	}

	sr.lnCtrl = ln

	return nil
}
