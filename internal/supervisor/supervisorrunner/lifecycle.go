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
	"bytes"
	"encoding/json"
	"errors"
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

func (sr *Exec) StartSessionCmd(session *api.SupervisedSession) error {
	sr.logger.Debug(
		"StartSessionCmd: preparing to start session",
		"session_id",
		session.Spec.ID,
		"command",
		session.Command,
		"commandArgs",
		session.CommandArgs,
	)
	// devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)

	if session.Command == "" || session.CommandArgs == nil {
		sr.logger.Error(
			"StartSessionCmd: command or command args are nil",
			"session_id",
			session.Spec.ID,
			"command",
			session.Command,
			"command_args",
			session.CommandArgs,
		)
		return fmt.Errorf("%w: command or command args are nil", errdefs.ErrSessionCmdStart)
	}
	//nolint:gosec,noctx // User has to specify the command and its args; we explicitly don't want to use context here to avoid killing the process on parent exit
	cmd := exec.Command(session.Command, session.CommandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	b, err := json.Marshal(session.Spec)
	if err != nil {
		sr.logger.Debug("failed to marshal session spec to JSON", "error", err)
	} else {
		sr.logger.Debug("session spec JSON", "session_spec", string(b))
	}

	cmd.Stdin = bytes.NewReader(b)

	// Inherit Environment Variables - Temporal solution for 'sbsh run' to be able to inherit env vars
	if session.Spec.EnvInherit {
		cmd.Env = os.Environ()
		sr.logger.Debug("StartSessionCmd: inheriting environment variables", "session_id", session.Spec.ID)
	} else {
		// sbsh run needs at least HOME in env
		home := os.Getenv("HOME")
		if home != "" {
			cmd.Env = []string{"HOME=" + home}
			sr.logger.Debug("StartSessionCmd: not inheriting environment variables, only HOME is set", "session_id", session.Spec.ID, "HOME", home)
		} else {
			sr.logger.Error("StartSessionCmd: not inheriting environment variables, HOME is not set", "session_id", session.Spec.ID)
			return errors.New("HOME environment variable is not set in parent environment; cannot start session without HOME")
		}
	}

	if errS := cmd.Start(); errS != nil {
		sr.logger.Error("StartSessionCmd: failed to start command", "session_id", session.Spec.ID, "error", errS)
		return fmt.Errorf("%w :%w", errdefs.ErrSessionCmdStart, errS)
	}

	// you can return cmd.Process.Pid to record in meta.json
	// session.Pid = cmd.Process.Pid
	sr.logger.Info("StartSessionCmd: process started", "session_id", session.Spec.ID, "pid", cmd.Process.Pid)

	// IMPORTANT: reap it in the background so it never zombifies
	go func() {
		errWait := cmd.Wait()
		if errWait != nil {
			sr.logger.Warn(
				"StartSessionCmd: process exited with error",
				"session_id",
				session.Spec.ID,
				"error",
				errWait,
			)
		} else {
			sr.logger.Info("StartSessionCmd: process exited", "session_id", session.Spec.ID)
		}
		eventErr := fmt.Errorf("session %s process has exited", session.Spec.ID)
		trySendEvent(
			sr.logger,
			sr.events,
			Event{ID: session.Spec.ID, Type: EvCmdExited, Err: eventErr, When: time.Now()},
		)
	}()

	return nil
}

func (sr *Exec) Attach(session *api.SupervisedSession) error {
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

	if err := sr.startConnectionManager(); err != nil {
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

func (sr *Exec) Close(_ error) error {
	sr.logger.Debug("Close: cancelling context and cleaning up")
	sr.ctxCancel()

	sr.metadata.Status.State = api.SupervisorExiting
	errM := sr.updateMetadata()
	if errM != nil {
		sr.logger.ErrorContext(sr.ctx, "failed to update metadata", "error", errM)
	}

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

	sr.metadata.Status.State = api.SupervisorExited
	errE := sr.updateMetadata()
	if errE != nil {
		sr.logger.ErrorContext(sr.ctx, "failed to update metadata", "error", errE)
	}
	return nil
}

func (sr *Exec) WaitClose(_ error) error {
	return nil
}

func (sr *Exec) Resize(_ api.ResizeArgs) {
	// No-op
}

func (sr *Exec) Detach() error {
	if err := sr.sessionClient.Detach(sr.ctx, &sr.id); err != nil {
		return err
	}

	sr.logger.Info("Supervisor detached", "supervisor_id", sr.id)
	if _, err := os.Stdout.WriteString("\x1b[93m\r\nDetached\x1b[0m\r\n"); err != nil {
		sr.logger.Warn("Failed to write detach message to stdout", "error", err)
		return err
	}

	return nil
}
