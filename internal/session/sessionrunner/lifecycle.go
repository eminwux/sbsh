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
	"path/filepath"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/pkg/api"
)

const deleteSessionDir bool = false

func (sr *Exec) StartSession(evCh chan<- Event) error {
	sr.logger.Info("starting session", "id", sr.id)
	if err := sr.updateSessionState(api.Initializing); err != nil {
		sr.logger.Error("failed to update session state on initializing", "id", sr.id, "err", err)
	}
	sr.evCh = evCh

	if err := sr.prepareSessionCommand(); err != nil {
		sr.logger.Error("failed to run session command", "id", sr.id, "err", err)
		return fmt.Errorf("failed to run session command for session %s: %w", sr.id, err)
	}

	if err := sr.startPty(); err != nil {
		sr.logger.Error("failed to start PTY", "id", sr.id, "err", err)
		return fmt.Errorf("failed to start PTY for session %s: %w", sr.id, err)
	}

	sr.logger.Info("session PTY started", "id", sr.id)
	go sr.waitOnSession()

	return nil
}

func (sr *Exec) waitOnSession() {
	err := <-sr.closeReqCh
	sr.logger.Info("session close requested", "id", sr.id, "err", err)
	sr.logger.Debug("sending EvSessionExited event", "id", sr.id)
	trySendEvent(sr.logger, sr.evCh, Event{ID: sr.id, Type: EvCmdExited, Err: err, When: time.Now()})
}

func (sr *Exec) Close(reason error) error {
	sr.logger.Info("closing session", "id", sr.id, "reason", reason)

	if err := sr.updateSessionState(api.Exited); err != nil {
		sr.logger.Error("failed to update session state", "id", sr.id, "err", err)
	}

	sr.ctxCancel()

	sr.logger.Debug("session closed", "id", sr.id)

	// stop accepting
	if sr.lnCtrl != nil {
		if err := sr.lnCtrl.Close(); err != nil {
			sr.logger.Warn("could not close IO listener", "id", sr.id, "err", err)
		}
	}

	// close clients
	sr.clientsMu.Lock()
	for _, c := range sr.clients {
		if err := c.conn.Close(); err != nil {
			sr.logger.Warn("could not close client connection", "id", sr.id, "client", c.id, "err", err)
		}
	}

	sr.clients = nil
	sr.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if sr.cmd != nil && sr.cmd.Process != nil {
		if err := sr.cmd.Process.Kill(); err != nil {
			sr.logger.Warn("could not kill cmd process", "id", sr.id, "err", err)
		}
	}
	if sr.ptmx != nil {
		var err error
		sr.closePTY.Do(func() {
			err = sr.ptmx.Close()
		})
		if err != nil {
			sr.logger.Warn("could not close pty", "id", sr.id, "err", err)
		}
	}

	// remove Ctrl socket
	if err := os.Remove(sr.metadata.Status.SocketFile); err != nil {
		sr.logger.Warn("couldn't remove Ctrl socket", "id", sr.id, "socket", sr.metadata.Status.SocketFile, "err", err)
	}

	if deleteSessionDir {
		if err := os.RemoveAll(filepath.Dir(sr.metadata.Status.SessionRunPath)); err != nil {
			sr.logger.Warn(
				"couldn't remove session directory",
				"id",
				sr.id,
				"dir",
				sr.metadata.Status.SessionRunPath,
				"err",
				err,
			)
		}
	}

	close(sr.closedCh)
	return nil
}

func (sr *Exec) Resize(args api.ResizeArgs) {
	_ = pty.Setsize(sr.ptmx, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
}

// Write writes bytes to the session PTY (used by controller or Smart executor).
func (sr *Exec) Write(p []byte) (int, error) {
	for i := range p {
		_, err := sr.ptmx.Write([]byte{p[i]})
		if err != nil {
			return i, err
		}
		time.Sleep(time.Microsecond)
	}
	return len(p), nil
}

func (sr *Exec) Attach(supervisorID *api.ID, response *api.ResponseWithFD) error {
	cliFD, err := sr.CreateNewClient(supervisorID)
	if err != nil {
		return err
	}

	payload := struct {
		OK bool `json:"ok"`
	}{OK: true}

	fds := []int{cliFD}

	sr.logger.Debug("Attach response", "id", supervisorID, "ok", payload.OK, "fds", fds)

	response.JSON = payload
	response.FDs = fds

	return nil
}

func (sr *Exec) Detach(id *api.ID) error {
	// 1) Lookup
	ioClient, okClient := sr.getClient(*id)
	if !okClient {
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
		//nolint:mnd // grace period
		time.Sleep(100 * time.Millisecond)             // allow `sb detach` to finish
		_ = conn.SetDeadline(time.Now())               // unblock any Read/Write
		if u, okConn := conn.(*net.UnixConn); okConn { // optional: half-close to flush
			_ = u.CloseWrite()
			_ = u.CloseRead()
		}
		_ = conn.Close()
	}()

	if err := sr.updateSessionAttachers(); err != nil {
		sr.logger.Warn("failed to update metadata on client cleanup", "err", err)
	}

	return nil
}

func (sr *Exec) SetupShell() error {
	sr.logger.Debug("setupShell: sending CTRL-U")
	if _, err := sr.Write([]byte("\x15")); err != nil {
		sr.logger.Error("failed to send CTRL-U", "id", sr.id, "err", err)
		return fmt.Errorf("failed to send CTRL-U: %w", err)
	}

	// sr.logger.Debug("setupShell: sending CTRL-L")
	// if _, err := sr.Write([]byte("\x0c")); err != nil {
	// 	sr.logger.Error("failed to send CTRL-L", "id", sr.id, "err", err)
	// 	return fmt.Errorf("failed to send CTRL-L: %w", err)
	// }

	// set up prompt
	var promptCmd string
	if sr.metadata.Spec.Prompt != "" {
		promptCmd = `export PS1=` + sr.metadata.Spec.Prompt + "\n"
	} else {
		promptCmd = `export PS1="(sbsh-` + string(sr.metadata.Spec.ID) + `) $PS1"` + "\n"
	}

	sr.logger.Debug("setupShell: setting prompt", "cmd", promptCmd)
	if _, err := sr.Write([]byte(promptCmd)); err != nil {
		sr.logger.Error("failed to set prompt", "id", sr.id, "cmd", promptCmd, "err", err)
		return fmt.Errorf("failed to set prompt: %w", err)
	}

	//nolint:mnd // small delay between commands
	time.Sleep(10 * time.Millisecond)

	// setup current working directory
	if sr.metadata.Spec.Cwd != "" {
		cmdLine := "cd " + sr.metadata.Spec.Cwd + "\n"
		sr.logger.Info("OnInit command", "cmd", cmdLine)

		if _, err := sr.Write([]byte(cmdLine)); err != nil {
			sr.logger.Error("failed to write OnInit command to PTY", "cmd", cmdLine, "err", err)
			return fmt.Errorf("failed to write OnInit command to PTY: %w", err)
		}

		//nolint:mnd // small delay between commands
		time.Sleep(10 * time.Millisecond)
	}

	if err := sr.updateSessionState(api.Ready); err != nil {
		sr.logger.Error("failed to update session state on ready", "id", sr.id, "err", err)
		return fmt.Errorf("failed to update session state on ready: %w", err)
	}

	return nil
}

func (sr *Exec) writeStage(stage *[]api.ExecStep) error {
	if stage != nil {
		// run OnInit steps
		for _, step := range *stage {
			cmdLine := ""
			for k, v := range step.Env {
				cmdLine += fmt.Sprintf("%s=%q ", k, v)
			}
			cmdLine += step.Script
			sr.logger.Info("stage command", "cmd", cmdLine)

			_, err := sr.Write([]byte(cmdLine + "\n"))
			if err != nil {
				sr.logger.Error("failed to write stage command to PTY", "cmd", cmdLine, "err", err)
				return fmt.Errorf("failed to write stage command to PTY: %w", err)
			}

			// small delay between commands
			//nolint:mnd // small delay between commands
			time.Sleep(10 * time.Millisecond)
		}
	}
	return nil
}

func (sr *Exec) PostAttachShell() error {
	if err := sr.writeStage(&sr.metadata.Spec.Stages.PostAttach); err != nil {
		return err
	}
	return nil
}

func (sr *Exec) OnInitShell() error {
	if err := sr.writeStage(&sr.metadata.Spec.Stages.OnInit); err != nil {
		return err
	}

	if err := sr.updateSessionState(api.Ready); err != nil {
		sr.logger.Error("failed to update session state on ready", "id", sr.id, "err", err)
	}
	return nil
}
