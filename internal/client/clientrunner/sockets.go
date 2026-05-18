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

package clientrunner

import (
	"fmt"
	"net"
	"os"
	"time"
)

// liveSocketProbeTimeout bounds the DialTimeout used by OpenSocketCtrl to
// decide whether a peer is already serving the control socket path before
// unlinking it. A successful dial inside this window means the inode is
// live and the bind must refuse; any dial failure (ENOENT, ECONNREFUSED,
// timeout) means no peer is accepting and the path is safe to unlink.
const liveSocketProbeTimeout = 100 * time.Millisecond

func (sr *Exec) OpenSocketCtrl() error {
	sr.metadataMu.RLock()
	socketCtrl := sr.metadata.Spec.SockerCtrl
	sr.metadataMu.RUnlock()

	sr.logger.Debug("OpenSocketCtrl: preparing to listen", "socket", socketCtrl)

	// Probe before unlinking: a successful dial means a live peer is
	// serving this path (operator passed the same --id / --socket-file
	// twice, two starts raced, an old process is still draining a slow
	// client). Refusing the bind keeps the victim's socket intact and
	// surfaces a clear diagnostic instead of silently breaking new
	// dials against the older client. A dial failure (ENOENT,
	// ECONNREFUSED, timeout) means no peer is accepting; the path is
	// safe to unlink.
	probeDialer := net.Dialer{Timeout: liveSocketProbeTimeout}
	if conn, errDial := probeDialer.DialContext(sr.ctx, "unix", socketCtrl); errDial == nil {
		_ = conn.Close()
		sr.logger.Error(
			"OpenSocketCtrl: socket already in use by live peer",
			"socket", socketCtrl,
		)
		return fmt.Errorf("socket already in use by live peer: %s", socketCtrl)
	}

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
