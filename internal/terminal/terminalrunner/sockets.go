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

package terminalrunner

import (
	"fmt"
	"net"
	"os"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// defaultSocketMode is the legacy owner-only mode applied when the spec
// leaves SocketMode at its zero value. Group/world access is opt-in via
// the --socket-mode flag, SBSH_TERM_SOCKET_MODE env, or the profile's
// spec.socket.mode field.
const defaultSocketMode os.FileMode = 0o600

// defaultArtifactMode is the legacy owner-only mode applied when the
// spec leaves CaptureMode / LogFileMode at their zero value. Group or
// world access is opt-in via the matching --capture-* / --log-file-*
// flags, env vars, or profile blocks. Same contract shape as
// defaultSocketMode for the control socket.
const defaultArtifactMode os.FileMode = 0o600

// applyArtifactPerms chmods (and optionally chowns the group of) a
// per-terminal artifact path that was just opened. Used for the capture
// transcript and the log file — the control socket has its own helper in
// OpenSocketCtrl because the path needs to be removed first and the
// listener has to be torn down on failure.
//
// mode == 0 falls back to defaultArtifactMode so callers can pass the
// raw spec field through without a guard. gid == nil leaves the group
// unchanged. The chown form is Chown(path, -1, gid) so the listener's
// uid stays the owner; only the group is rewritten.
func (sr *Exec) applyArtifactPerms(label, path string, mode os.FileMode, gid *int) error {
	if mode == 0 {
		mode = defaultArtifactMode
	}
	if errChmod := os.Chmod(path, mode); errChmod != nil {
		sr.logger.Error(
			"applyArtifactPerms: chmod failed",
			"artifact", label,
			"path", path,
			"mode", fmt.Sprintf("0o%o", mode),
			"error", errChmod,
		)
		return fmt.Errorf("chmod %s: %w", label, errChmod)
	}
	if gid != nil {
		if errChown := os.Chown(path, -1, *gid); errChown != nil {
			sr.logger.Error(
				"applyArtifactPerms: chown failed",
				"artifact", label,
				"path", path,
				"gid", *gid,
				"error", errChown,
			)
			return fmt.Errorf("chown %s: %w", label, errChown)
		}
	}
	return nil
}

func (sr *Exec) OpenSocketCtrl() error {
	sr.metadataMu.Lock()
	socketFile := sr.metadata.Spec.SocketFile
	socketMode := sr.metadata.Spec.SocketMode
	var socketGID *int
	if g := sr.metadata.Spec.SocketGID; g != nil {
		gv := *g
		socketGID = &gv
	}
	sr.metadata.Status.SocketFile = socketFile
	sr.metadataMu.Unlock()

	if socketMode == 0 {
		socketMode = defaultSocketMode
	}

	sr.logger.Debug("OpenSocketCtrl: preparing to listen", "socket", socketFile)
	errMetadata := sr.updateMetadata()
	if errMetadata != nil {
		sr.logger.Error("OpenSocketCtrl: failed to update metadata", "error", errMetadata)
		return fmt.Errorf("update metadata: %w", errMetadata)
	}

	// Remove sockets if they already exist
	if err := os.Remove(socketFile); err != nil {
		sr.logger.Warn(
			"OpenSocketCtrl: couldn't remove stale CTRL socket",
			"socket",
			socketFile,
			"error",
			err,
		)
	}

	// Listen to CONTROL SOCKET
	var lc net.ListenConfig
	ctrlLn, errListen := lc.Listen(sr.ctx, "unix", socketFile)
	if errListen != nil {
		sr.logger.Error("OpenSocketCtrl: failed to listen", "socket", socketFile, "error", errListen)
		return fmt.Errorf("listen ctrl: %w", errListen)
	}

	// keep references for Close()
	sr.lnCtrl = ctrlLn

	if errChmod := os.Chmod(socketFile, socketMode); errChmod != nil {
		sr.logger.Error(
			"OpenSocketCtrl: failed to chmod socket file",
			"socket",
			socketFile,
			"mode",
			fmt.Sprintf("0o%o", socketMode),
			"error",
			errChmod,
		)
		_ = ctrlLn.Close()
		return errChmod
	}

	if socketGID != nil {
		// Chown(-1, gid) keeps the listener's uid as owner and only
		// rewrites the group, so the inode permits group-mode access
		// without changing the controlling identity.
		if errChown := os.Chown(socketFile, -1, *socketGID); errChown != nil {
			sr.logger.Error(
				"OpenSocketCtrl: failed to chown socket file",
				"socket",
				socketFile,
				"gid",
				*socketGID,
				"error",
				errChown,
			)
			_ = ctrlLn.Close()
			return errChown
		}
	}

	// update metadata with socket file path
	sr.metadataMu.Lock()
	sr.metadata.Status.SocketFile = socketFile
	sr.metadataMu.Unlock()
	if err := sr.updateMetadata(); err != nil {
		sr.logger.Error("OpenSocketCtrl: failed to update metadata", "error", err)
		_ = ctrlLn.Close()
		return err
	}

	sr.logger.Info("OpenSocketCtrl: listening on socket", "socket", socketFile)
	return nil
}

func (sr *Exec) CreateNewClient(clientID *api.ID) (int, error) {
	sr.logger.Debug("CreateNewClient: creating socketpair", "id", clientID)
	sv, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|sockCloexec, 0)
	if err != nil {
		sr.logger.Error("CreateNewClient: Socketpair failed", "id", clientID, "error", err)
		return -1, err
	}

	srvFD := sv[0]
	cliFD := sv[1]

	f := os.NewFile(uintptr(srvFD), "terminal-io")

	ioConn, err := net.FileConn(f)
	if err != nil {
		sr.logger.Error("CreateNewClient: FileConn failed", "id", clientID, "error", err)
		return -1, fmt.Errorf("FileConn: %w", err)
	}
	_ = f.Close() // release the duplicate, keep using ioConn

	cl := &ioClient{id: clientID, conn: ioConn}

	sr.addClient(cl)
	sr.logger.Info("CreateNewClient: client added", "id", clientID)
	go sr.handleClient(cl)

	sr.logger.Debug("CreateNewClient: returning client FD", "id", clientID, "fd", cliFD)
	return cliFD, nil
}

func (sr *Exec) getClientList() []*api.ID {
	sr.clientsMu.Lock()
	defer sr.clientsMu.Unlock()

	ids := make([]*api.ID, 0, len(sr.clients))
	for _, c := range sr.clients {
		ids = append(ids, c.id)
	}
	return ids
}
