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
	"path/filepath"
	"time"

	"github.com/creack/pty"
	"sbsh/pkg/api"
)

const deleteSessionDir bool = false

func (sr *SessionRunnerExec) StartSession(evCh chan<- SessionRunnerEvent) error {
	sr.evCh = evCh

	if err := sr.openSocketIO(); err != nil {
		err = fmt.Errorf("failed to open IO socket for session %s: %w", sr.id, err)
		return err
	}

	if err := sr.prepareSessionCommand(); err != nil {
		err = fmt.Errorf("failed to run session command for session %s: %v", sr.id, err)
		return err
	}

	if err := sr.startPTY(); err != nil {
		err = fmt.Errorf("failed to start PTY for session %s: %v", sr.id, err)
		return err
	}

	go sr.waitOnSession()

	return nil
}

func (sr *SessionRunnerExec) waitOnSession() {
	select {
	case err := <-sr.closeReqCh:
		slog.Debug("[session] sending EvSessionExited event\r\n")
		trySendEvent(sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvCmdExited, Err: err, When: time.Now()})
		return
	}
}

func (sr *SessionRunnerExec) Close(reason error) error {
	slog.Debug(fmt.Sprintf("[session-runner] closing session-runner on request, reason: %v\r\n", reason))

	sr.metadata.Status.State = api.SessionStatusExited
	sr.updateMetadata()

	sr.ctxCancel()

	slog.Debug("[session-runner] closed \r\n")

	// stop accepting
	if sr.listenerCtrl != nil {
		if err := sr.listenerCtrl.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close IO listener: %v", err))
		}
	}
	// stop accepting
	if sr.listenerIO != nil {
		if err := sr.listenerIO.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close IO listener: %v", err))
		}
	}

	// close clients
	sr.clientsMu.Lock()
	for _, c := range sr.clients {
		if err := c.conn.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close connection: %v\r\n", err))
		}
	}

	sr.clients = nil
	sr.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if sr.cmd != nil && sr.cmd.Process != nil {
		if err := sr.cmd.Process.Kill(); err != nil {
			slog.Debug(fmt.Sprintf("[sesion] could not kill cmd: %v\r\n", err))
		}
	}
	if sr.pty != nil {
		var err error
		closePTY.Do(func() {
			err = sr.pty.Close()
		})
		if err != nil {
			slog.Debug(fmt.Sprintf("[sesion] could not close pty: %v\r\n", err))
		}
	}

	// remove sockets and dir
	if err := os.Remove(sr.socketIO); err != nil {
		slog.Debug(fmt.Sprintf("[session] couldn't remove IO socket: %s: %v\r\n", sr.socketIO, err))
	}

	// remove Ctrl socket
	if err := os.Remove(sr.socketCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[sessionCtrl] couldn't remove Ctrl socket %s: %v\r\n", sr.socketCtrl, err))
	}

	if deleteSessionDir {
		if err := os.RemoveAll(filepath.Dir(sr.socketIO)); err != nil {
			slog.Debug(fmt.Sprintf("[session] couldn't remove session directory '%s': %v\r\n", sr.socketIO, err))
		}
	}

	close(sr.closedCh)
	return nil
}

func (sr *SessionRunnerExec) Resize(args api.ResizeArgs) {
	pty.Setsize(sr.pty, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
}

// Write writes bytes to the session PTY (used by controller or Smart executor).
func (sr *SessionRunnerExec) Write(p []byte) (int, error) {
	for i := range p {
		_, err := sr.pty.Write([]byte{p[i]})
		if err != nil {
			return i, err
		}
		time.Sleep(time.Microsecond)
	}
	return len(p), nil
}

func (sr *SessionRunnerExec) Attach(id *api.ID, response *api.ResponseWithFD) error {
	cliFD, err := sr.CreateNewClient(id)
	if err != nil {
		return err
	}

	payload := struct {
		OK bool `json:"ok"`
	}{OK: true}

	fds := []int{cliFD}

	slog.Debug("[session] Attach response",
		"ok", payload.OK,
		"fds", fds,
	)

	response.JSON = payload
	response.FDs = fds

	return nil
}

func (sr *SessionRunnerExec) Detach(id *api.ID) error {
	// 1) Lookup
	ioClient, ok := sr.getClient(*id)
	if !ok {
		return fmt.Errorf("supervisor %s not found among clients", *id)
	}

	// 2) Remove from fan-out paths first so no more writes target it
	sr.ptyPipes.multiOutW.Remove(ioClient.pipeOutW)
	// TODO: remove from stdin fan-in if you have that side too
	// sr.ptyPipes.multiInR.Remove(ioClient.pipeInR)

	// 3) Drop from the registry so it’s no longer discoverable
	sr.removeClient(ioClient)

	// 4) Close the conn asynchronously after a short grace period
	conn := ioClient.conn
	go func() {
		time.Sleep(100 * time.Millisecond)     // allow `sb detach` to finish
		_ = conn.SetDeadline(time.Now())       // unblock any Read/Write
		if u, ok := conn.(*net.UnixConn); ok { // optional: half-close to flush
			_ = u.CloseWrite()
			_ = u.CloseRead()
		}
		_ = conn.Close()
	}()

	sr.metadata.Status.State = api.SessionStatusDetached
	_ = sr.updateMetadata()

	return nil
}
