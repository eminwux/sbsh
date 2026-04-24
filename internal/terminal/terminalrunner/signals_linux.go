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

//go:build linux

package terminalrunner

import (
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// forwardedSignals is the minimum set the PID-1 forwarder relays to the
// tracked child's process group. SIGWINCH is intentionally omitted: PTY
// resize already has its own dedicated path.
var forwardedSignals = []os.Signal{
	syscall.SIGTERM,
	syscall.SIGINT,
	syscall.SIGHUP,
	syscall.SIGQUIT,
}

// startSignalForwarder installs a goroutine that forwards forwardedSignals to
// the child's process group (negative pid). pgid must be positive; the
// forwarder negates it before syscall.Kill. Returns a stop function; safe to
// call multiple times.
func startSignalForwarder(logger *slog.Logger, pgid int) func() {
	if pgid <= 0 {
		logger.Warn("signal forwarder: invalid pgid, skipping", "pgid", pgid)
		return func() {}
	}

	sigCh := make(chan os.Signal, len(forwardedSignals))
	signal.Notify(sigCh, forwardedSignals...)

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				sysSig, castOK := sig.(syscall.Signal)
				if !castOK {
					logger.Warn("signal forwarder: non-syscall signal", "sig", sig)
					continue
				}
				if err := syscall.Kill(-pgid, sysSig); err != nil && !errors.Is(err, syscall.ESRCH) {
					logger.Warn("signal forwarder: kill failed",
						"pgid", pgid, "sig", sysSig, "err", err)
					continue
				}
				logger.Debug("signal forwarder: relayed signal", "pgid", pgid, "sig", sysSig)
			}
		}
	}()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			signal.Stop(sigCh)
			close(stop)
			<-done
		})
	}
}
