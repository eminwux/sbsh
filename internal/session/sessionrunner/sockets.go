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
	"net"
	"os"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

func (sr *SessionRunnerExec) OpenSocketCtrl() error {
	sr.logger.Debug("OpenSocketCtrl: preparing to listen", "socket", sr.metadata.Spec.SocketFile)
	sr.metadata.Status.SocketFile = sr.metadata.Spec.SocketFile
	sr.updateMetadata()

	// Remove sockets if they already exist
	if err := os.Remove(sr.metadata.Spec.SocketFile); err != nil {
		sr.logger.Warn(
			"OpenSocketCtrl: couldn't remove stale CTRL socket",
			"socket",
			sr.metadata.Spec.SocketFile,
			"error",
			err,
		)
	}

	// Listen to CONTROL SOCKET
	var lc net.ListenConfig
	ctrlLn, err := lc.Listen(sr.ctx, "unix", sr.metadata.Spec.SocketFile)
	if err != nil {
		sr.logger.Error("OpenSocketCtrl: failed to listen", "socket", sr.metadata.Spec.SocketFile, "error", err)
		return fmt.Errorf("listen ctrl: %w", err)
	}

	// keep references for Close()
	sr.lnCtrl = ctrlLn

	if err := os.Chmod(sr.metadata.Spec.SocketFile, 0o600); err != nil {
		sr.logger.Error(
			"OpenSocketCtrl: failed to chmod socket file",
			"socket",
			sr.metadata.Spec.SocketFile,
			"error",
			err,
		)
		ctrlLn.Close()
		return err
	}

	// update metadata with socket file path
	sr.metadata.Status.SocketFile = sr.metadata.Spec.SocketFile
	sr.updateMetadata()

	sr.logger.Info("OpenSocketCtrl: listening on socket", "socket", sr.metadata.Spec.SocketFile)
	return nil
}

func (sr *SessionRunnerExec) CreateNewClient(supervisorID *api.ID) (int, error) {
	sr.logger.Debug("CreateNewClient: creating socketpair", "id", supervisorID)
	sv, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|sockCloexec, 0)
	if err != nil {
		sr.logger.Error("CreateNewClient: Socketpair failed", "id", supervisorID, "error", err)
		return -1, err
	}

	srvFD := sv[0]
	cliFD := sv[1]

	f := os.NewFile(uintptr(srvFD), "session-io")

	ioConn, err := net.FileConn(f)
	if err != nil {
		sr.logger.Error("CreateNewClient: FileConn failed", "id", supervisorID, "error", err)
		return -1, fmt.Errorf("FileConn: %w", err)
	}
	_ = f.Close() // release the duplicate, keep using ioConn

	cl := &ioClient{id: supervisorID, conn: ioConn}

	sr.addClient(cl)
	sr.logger.Info("CreateNewClient: client added", "id", supervisorID)
	go sr.handleClient(cl)

	sr.logger.Debug("CreateNewClient: returning client FD", "id", supervisorID, "fd", cliFD)
	return cliFD, nil
}

func (sr *SessionRunnerExec) getClientList() []*api.ID {
	sr.clientsMu.Lock()
	defer sr.clientsMu.Unlock()

	ids := make([]*api.ID, 0, len(sr.clients))
	for _, c := range sr.clients {
		ids = append(ids, c.id)
	}
	return ids
}
