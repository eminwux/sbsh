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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/cmd/config"
)

var closePTY sync.Once

// closePTY is used to ensure PTY is closed only once (shared across session lifecycle).
func (sr *SessionRunnerExec) prepareSessionCommand() error {
	// Build the child command with context (so ctx cancel can kill it)
	cmd := exec.CommandContext(sr.ctx, sr.metadata.Spec.Command, sr.metadata.Spec.CommandArgs...)
	// Environment: use provided or inherit
	if len(sr.metadata.Spec.Env) > 0 {
		cmd.Env = sr.metadata.Spec.Env
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env,
		config.KV(config.SES_SOCKET_CTRL, sr.metadata.Status.SocketFile),
		config.KV(config.SES_ID, string(sr.metadata.Spec.ID)),
		config.KV(config.SES_NAME, sr.metadata.Spec.Name),
		config.KV(config.SES_PROFILE, sr.metadata.Spec.ProfileName),
	)
	// Start the process in a new session so it has its own process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true, // make the child the controlling TTY
		Setsid:  true, // new session
	}

	// Make sure TERM is reasonable if not set (helps colors)
	hasTERM := false
	for _, e := range cmd.Env {
		if len(e) >= 5 && e[:5] == "TERM=" {
			hasTERM = true
			break
		}
	}
	if !hasTERM {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color", "COLORTERM=truecolor")
	}

	sr.cmd = cmd

	return nil
}

func (sr *SessionRunnerExec) startPTY() error {
	// Start under a PTY and inherit current terminal size
	ptmx, err := pty.Start(sr.cmd)
	if err != nil {
		return err
	}
	sr.pty = ptmx

	go func() {
		sr.logger.Debug("waiting on child process", "parent_pid", os.Getpid(), "child_pid", sr.cmd.Process.Pid)
		_ = sr.cmd.Wait() // blocks until process exits
		sr.logger.Info("child process has exited", "parent_pid", os.Getpid(), "child_pid", sr.cmd.Process.Pid)
		sr.closeReqCh <- errors.New("the shell process has exited")
	}()

	logf, err := os.OpenFile(sr.metadata.Spec.CaptureFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	if err := sr.updateMetadata(); err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}

	// StdIn
	// PTY reads from pipeInR
	// conn writes to pipeInW
	pipeInR, pipeInW, err := os.Pipe()
	if err != nil {
		sr.logger.Error("error opening IN pipe", "err", err)
		return fmt.Errorf("error opening IN pipe: %w", err)
	}

	// StdOut
	// ATTACHED: stream to client (pipeOutW) AND log file
	multiOutW := NewDynamicMultiWriter(sr.logger, logf)

	sr.ptyPipes.pipeInR = pipeInR
	sr.ptyPipes.pipeInW = pipeInW
	sr.ptyPipes.multiOutW = multiOutW

	go sr.terminalManager(pipeInR, multiOutW)

	return nil
}

func (sr *SessionRunnerExec) terminalManager(pipeInR *os.File, multiOutW io.Writer) error {
	/*
	 * PTY READER goroutine
	 */

	go func() {
		sr.terminalManagerReader(multiOutW)
	}()

	/*
	* PTY WRITER goroutine
	 */
	go func() {
		sr.terminalManagerWriter(pipeInR)
	}()

	return nil
}

func (sr *SessionRunnerExec) terminalManagerReader(multiOutW io.Writer) error {
	go func() {
		<-sr.ctx.Done()
		sr.logger.Debug("finishing terminalManagerReader")
		closePTY.Do(func() { _ = sr.pty.Close() }) // unblocks Read
		sr.logger.Debug("finished terminalManagerReader")
	}()

	//nolint:mnd // buffer size
	buf := make([]byte, 8192)
	for {
		// READ FROM PTY - WRITE TO PIPE
		n, err := sr.pty.Read(buf)

		// drain/emit data
		if n > 0 {
			sr.lastRead = time.Now()
			sr.bytesOut += uint64(n)

			// Render if output is enabled; otherwise we just drain
			if sr.gates.OutputOn {
				sr.logger.Debug("read from pty", "bytes", n)
				sr.logger.Debug("writing to multiOutW")
				_, err := multiOutW.Write(buf[:n])
				sr.logger.Debug("writing to multiOutW done")
				if err != nil {
					sr.logger.Error("error writing raw data to client", "err", err)
					return ErrPipeWrite
				}
			}
		}

		// Handle read end/error
		if err != nil {
			// treat EOF as info, others as error
			if err == io.EOF {
				sr.logger.Info("pty stdout closed (EOF)")
			} else {
				sr.logger.Error("pty stdout error", "err", err)
			}
			trySendEvent(sr.logger, sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvError, Err: err, When: time.Now()})
			return ErrTerminalRead
		}
	}
}

func (sr *SessionRunnerExec) terminalManagerWriter(pipeInR *os.File) error {
	go func() {
		<-sr.ctx.Done()
		sr.logger.Debug("finishing terminalManagerWriter")
		closePTY.Do(func() { _ = sr.pty.Close() }) // unblocks Read
		_ = pipeInR.Close()
		sr.logger.Debug("finished terminalManagerWriter")
	}()

	//nolint:mnd // buffer size
	buf := make([]byte, 4096)
	i := 0
	for {
		// READ FROM PIPE - WRITE TO PTY
		// PTY reads from pipeInR
		sr.logger.Debug("reading from pipeInR", "iteration", i)
		i++
		n, err := pipeInR.Read(buf)
		sr.logger.Debug("read from pipeInR", "bytes", n)
		if n > 0 {
			if sr.gates.StdinOpen {
				sr.logger.Debug("reading from pipeInR (StdinOpen)")
				if _, werr := sr.pty.Write(buf[:n]); werr != nil {
					sr.logger.Error("error writing to PTY", "err", werr)
					trySendEvent(
						sr.logger,
						sr.evCh,
						SessionRunnerEvent{ID: sr.id, Type: EvError, Err: werr, When: time.Now()},
					)
					return ErrPipeRead
				}
			}
			// else: gate closed, drop input
		}

		if err != nil {
			if err == io.EOF {
				sr.logger.Info("pipeInR closed (EOF)")
			} else {
				sr.logger.Error("stdin error", "err", err)
			}
			trySendEvent(sr.logger, sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvError, Err: err, When: time.Now()})
			return ErrTerminalWrite
		}
	}
}
