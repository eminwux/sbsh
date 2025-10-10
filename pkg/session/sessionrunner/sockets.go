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
	"net"
	"os"

	"golang.org/x/sys/unix"
	"sbsh/pkg/api"
)

func (sr *SessionRunnerExec) OpenSocketCtrl() error {
	slog.Debug(fmt.Sprintf("[sessionCtrl] socket: %s", sr.metadata.Spec.SocketFile))
	sr.metadata.Status.SocketFile = sr.metadata.Spec.SocketFile
	sr.updateMetadata()

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(sr.metadata.Spec.SocketFile); err != nil {
		slog.Debug(fmt.Sprintf("[sessionCtrl] couldn't remove stale CTRL socket: %s\r\n", sr.metadata.Spec.SocketFile))
	}

	// Listen to CONTROL SOCKET
	var lc net.ListenConfig
	ctrlLn, err := lc.Listen(sr.ctx, "unix", sr.metadata.Spec.SocketFile)
	if err != nil {
		return fmt.Errorf("listen ctrl: %w", err)
	}

	// keep references for Close()
	sr.lnCtrl = ctrlLn

	if err := os.Chmod(sr.metadata.Spec.SocketFile, 0o600); err != nil {
		ctrlLn.Close()
		return err
	}

	// update metadata with socket file path
	sr.metadata.Status.SocketFile = sr.metadata.Spec.SocketFile
	sr.updateMetadata()

	return nil
}

func (sr *SessionRunnerExec) CreateNewClient(id *api.ID) (int, error) {
	var sv [2]int
	var err error

	sv, err = unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|sockCloexec, 0)
	if err != nil {
		return -1, err
	}

	srvFD := sv[0]
	cliFD := sv[1]

	f := os.NewFile(uintptr(srvFD), "session-io")

	ioConn, err := net.FileConn(f)
	if err != nil {
		return -1, fmt.Errorf("FileConn: %w", err)
	}
	f.Close() // release the duplicate, keep using ioConn

	cl := &ioClient{id: id, conn: ioConn}

	sr.addClient(cl)
	go sr.handleClient(cl)

	return cliFD, nil
}
