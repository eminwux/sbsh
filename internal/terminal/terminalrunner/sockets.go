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
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// liveSocketProbeTimeout bounds the DialTimeout used by OpenSocketCtrl to
// decide whether a peer is already serving the control socket path before
// unlinking it. A successful dial inside this window means the inode is
// live and the bind must refuse; any dial failure (ENOENT, ECONNREFUSED,
// timeout) means no peer is accepting and the path is safe to unlink.
const liveSocketProbeTimeout = 100 * time.Millisecond

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

// listenerUmaskMu serializes the umask save/set/Listen/restore sequence
// in listenUnixWithMode against in-package concurrent callers. umask(2)
// is process-wide (it lives in the kernel's shared fs_struct on Linux),
// so two concurrent OpenSocketCtrl callers without this mutex could
// observe each other's temporary umask. The mutex does not isolate the
// brief Listen window from file creations in *other* packages that
// share this process — that's an inherent cost of using umask to plug
// the bind-then-chmod EACCES window. The alternative (bind-on-side-path
// then rename(2)) relies on a kernel quirk for AF_UNIX sockets and is
// worse on portability and on the ENOENT-vs-EACCES dial-error story.
//
//nolint:gochecknoglobals // process-wide invariant guard, like the umask it serializes
var listenerUmaskMu sync.Mutex

// listenUnixWithMode binds an AF_UNIX listener with the process umask
// temporarily set so the socket inode is born at the configured mode,
// not at (0o666 & ~daemonUmask). This closes the bind-then-chmod
// EACCES window in OpenSocketCtrl: pre-fix, the socket was briefly
// reachable at the daemon's restrictive default (typically 0o600 under
// a 0o077 umask), so a group-member client that dialed before
// applySocketPerms landed hit EACCES even though the final post-chmod
// state would have admitted it.
//
// runtime.LockOSThread pins the goroutine so the save/restore sequence
// runs on a single OS thread; listenerUmaskMu serializes the temporary
// umask against in-package concurrent callers. Callers should still
// run applySocketPerms after this returns — the chmod becomes
// idempotent at that point, but the chown still needs to apply when
// SocketGID is set and the parent directory does not propagate gid.
func listenUnixWithMode(ctx context.Context, socketFile string, mode os.FileMode) (net.Listener, error) {
	if mode == 0 {
		mode = defaultSocketMode
	}
	listenerUmaskMu.Lock()
	defer listenerUmaskMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	//nolint:mnd // 0o777 is the standard permission-bit mask for umask(2)
	prev := unix.Umask(int(^mode & 0o777))
	defer unix.Umask(prev)
	var lc net.ListenConfig
	return lc.Listen(ctx, "unix", socketFile)
}

// applySocketPerms chmods (and optionally chowns the group of) the
// control-socket inode after a listener has been bound to it. Called
// from both OpenSocketCtrl (runner binds the socket itself) and
// UseListener (caller hands in a pre-bound listener — pkg/terminal/server
// facade case), so both paths land at the same on-disk permissions.
//
// mode == 0 falls back to defaultSocketMode so callers can pass the
// raw spec field through without a guard. gid == nil leaves the group
// unchanged. The chown form is Chown(path, -1, gid) so the listener's
// uid stays the owner; only the group is rewritten.
func (sr *Exec) applySocketPerms(path string, mode os.FileMode, gid *int) error {
	if mode == 0 {
		mode = defaultSocketMode
	}
	if errChmod := os.Chmod(path, mode); errChmod != nil {
		sr.logger.Error(
			"applySocketPerms: chmod failed",
			"socket", path,
			"mode", fmt.Sprintf("0o%o", mode),
			"error", errChmod,
		)
		return fmt.Errorf("chmod socket: %w", errChmod)
	}
	if gid != nil {
		if errChown := os.Chown(path, -1, *gid); errChown != nil {
			sr.logger.Error(
				"applySocketPerms: chown failed",
				"socket", path,
				"gid", *gid,
				"error", errChown,
			)
			return fmt.Errorf("chown socket: %w", errChown)
		}
	}
	return nil
}

// applyArtifactPerms chmods (and optionally chowns the group of) a
// per-terminal artifact path that was just opened. Used for the capture
// transcript and the log file — the control socket goes through
// applySocketPerms instead because the listener has to be torn down on
// failure.
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

	sr.logger.Debug("OpenSocketCtrl: preparing to listen", "socket", socketFile)
	errMetadata := sr.updateMetadata()
	if errMetadata != nil {
		sr.logger.Error("OpenSocketCtrl: failed to update metadata", "error", errMetadata)
		return fmt.Errorf("update metadata: %w", errMetadata)
	}

	// Probe before unlinking: a successful dial means a live peer is
	// serving this path (operator passed the same --id / --socket-file
	// twice, two starts raced, an old process is still draining a slow
	// client). Refusing the bind keeps the victim's socket intact and
	// surfaces a clear diagnostic instead of silently breaking attach
	// against the older terminal. A dial failure (ENOENT, ECONNREFUSED,
	// timeout) means no peer is accepting; the path is safe to unlink.
	probeDialer := net.Dialer{Timeout: liveSocketProbeTimeout}
	if conn, errDial := probeDialer.DialContext(sr.ctx, "unix", socketFile); errDial == nil {
		_ = conn.Close()
		sr.logger.Error(
			"OpenSocketCtrl: socket already in use by live peer",
			"socket", socketFile,
		)
		return fmt.Errorf("socket already in use by live peer: %s", socketFile)
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

	// Listen to CONTROL SOCKET — bind with the umask set so the
	// inode is born at socketMode and there is no window during
	// which a group-member client dialing the socket hits EACCES
	// because the daemon's umask masked off group access. See #361
	// and listenUnixWithMode for the full rationale.
	ctrlLn, errListen := listenUnixWithMode(sr.ctx, socketFile, socketMode)
	if errListen != nil {
		sr.logger.Error("OpenSocketCtrl: failed to listen", "socket", socketFile, "error", errListen)
		return fmt.Errorf("listen ctrl: %w", errListen)
	}

	// keep references for Close()
	sr.lnCtrl = ctrlLn

	if errPerms := sr.applySocketPerms(socketFile, socketMode, socketGID); errPerms != nil {
		_ = ctrlLn.Close()
		return errPerms
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

func (sr *Exec) CreateNewClient(clientID *api.ID, fullCapture, clearScreen bool) (int, error) {
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

	cl := &ioClient{id: clientID, conn: ioConn, fullCapture: fullCapture, clearScreen: clearScreen}

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
