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
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

const (
	deleteSupervisorDir bool = false
)

func (sr *SupervisorRunnerExec) StartSessionCmd(session *api.SupervisedSession) error {
	sr.logger.Debug("StartSessionCmd: preparing to start session", "session_id", session.Id, "command", session.Command)
	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(session.Command, session.CommandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull

	cmd.Env = session.Env

	if err := cmd.Start(); err != nil {
		sr.logger.Error("StartSessionCmd: failed to start command", "session_id", session.Id, "error", err)
		return fmt.Errorf("%w :%w", errdefs.ErrSessionCmdStart, err)
	}

	// you can return cmd.Process.Pid to record in meta.json
	session.Pid = cmd.Process.Pid
	sr.logger.Info("StartSessionCmd: process started", "session_id", session.Id, "pid", session.Pid)

	// IMPORTANT: reap it in the background so it never zombifies
	go func() {
		err := cmd.Wait()
		if err != nil {
			sr.logger.Warn("StartSessionCmd: process exited with error", "session_id", session.Id, "error", err)
		} else {
			sr.logger.Info("StartSessionCmd: process exited", "session_id", session.Id)
		}
		eventErr := fmt.Errorf("session %s process has exited", session.Id)
		trySendEvent(
			sr.logger,
			sr.events,
			SupervisorRunnerEvent{ID: session.Id, Type: EvCmdExited, Err: eventErr, When: time.Now()},
		)
	}()

	return nil
}

func (sr *SupervisorRunnerExec) Attach(session *api.SupervisedSession) error {
	sr.session = session

	if err := sr.dialSessionCtrlSocket(); err != nil {
		return err
	}

	if err := sr.attach(); err != nil {
		return err
	}

	if err := sr.forwardResize(); err != nil {
		return err
	}

	if err := sr.attachIOSocket(); err != nil {
		return err
	}

	if err := sr.waitReady(); err != nil {
		return err
	}

	if err := sr.initTerminal(); err != nil {
		return err
	}

	return nil
}

func (sr *SupervisorRunnerExec) Close(_ error) error {
	sr.logger.Debug("Close: cancelling context and cleaning up")
	sr.ctxCancel()
	// remove sockets and dir
	if err := os.Remove(sr.metadata.Spec.SockerCtrl); err != nil {
		sr.logger.Warn("Close: couldn't remove Ctrl socket", "socket", sr.metadata.Spec.SockerCtrl, "error", err)
	} else {
		sr.logger.Info("Close: removed Ctrl socket", "socket", sr.metadata.Spec.SockerCtrl)
	}

	if deleteSupervisorDir {
		dir := filepath.Dir(sr.metadata.Spec.SockerCtrl)
		if err := os.RemoveAll(dir); err != nil {
			sr.logger.Warn("Close: couldn't remove socket directory", "dir", dir, "error", err)
		} else {
			sr.logger.Info("Close: removed socket directory", "dir", dir)
		}
	}

	_ = sr.toExitShell()
	sr.logger.Debug("Close: cleanup complete")
	return nil
}

func (sr *SupervisorRunnerExec) WaitClose(_ error) error {
	return nil
}

func (sr *SupervisorRunnerExec) Resize(_ api.ResizeArgs) {
	// No-op
}

func (sr *SupervisorRunnerExec) Detach() error {
	if err := sr.sessionClient.Detach(sr.ctx, &sr.id); err != nil {
		return err
	}

	return nil
}
