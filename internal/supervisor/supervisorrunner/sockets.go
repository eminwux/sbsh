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
	"net"
	"os"
)

func (sr *Exec) OpenSocketCtrl() error {
	sr.metadataMu.RLock()
	socketCtrl := sr.metadata.Spec.SockerCtrl
	sr.metadataMu.RUnlock()

	sr.logger.Debug("OpenSocketCtrl: preparing to listen", "socket", socketCtrl)

	// remove stale socket if it exists
	if _, err := os.Stat(socketCtrl); err == nil {
		sr.logger.Warn("OpenSocketCtrl: removing stale socket", "socket", socketCtrl)
		if rmErr := os.Remove(socketCtrl); rmErr != nil {
			sr.logger.Error(
				"OpenSocketCtrl: failed to remove stale socket",
				"socket",
				socketCtrl,
				"error",
				rmErr,
			)
			return fmt.Errorf("cannot remove stale socket: %w", rmErr)
		}
	}
	lnCfg := net.ListenConfig{}
	ln, err := lnCfg.Listen(sr.ctx, "unix", socketCtrl)
	if err != nil {
		sr.logger.Error("OpenSocketCtrl: cannot listen", "socket", socketCtrl, "error", err)
		return fmt.Errorf("cannot listen: %w", err)
	}

	sr.lnCtrl = ln
	sr.logger.Info("OpenSocketCtrl: listening on socket", "socket", socketCtrl)
	return nil
}
