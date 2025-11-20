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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/cmd/config"
)

const (
	vt100Rows = 24
	vt100Cols = 80
)

// closePTY is used to ensure PTY is closed only once (shared across terminal lifecycle).
func (sr *Exec) prepareTerminalCommand() error {
	// Prepare the exec.Cmd based on terminal metadata
	//nolint:gosec // User has to specify the command and its args
	cmd := exec.CommandContext(sr.ctx, sr.metadata.Spec.Command, sr.metadata.Spec.CommandArgs...)

	// Inherit Environment Variables
	if sr.metadata.Spec.EnvInherit {
		sr.logger.Debug("inheriting environment variables from parent process")
		cmd.Env = append(cmd.Env, os.Environ()...)
	} else {
		sr.logger.Debug("only HOME will be inherited from parent environment")
		home := os.Getenv("HOME")
		sr.logger.Debug("setting HOME environment variable", "HOME", home)

		if home != "" {
			cmd.Env = append(cmd.Env, "HOME="+home)
		} else {
			sr.logger.Warn("HOME environment variable is not set; not appending HOME to child environment")
			return errors.New("HOME environment variable is not set in parent process; cannot start terminal without HOME")
		}
	}

	// Add custom env vars
	if len(sr.metadata.Spec.Env) > 0 {
		sr.logger.Debug("adding custom environment variables", "env_count", len(sr.metadata.Spec.Env))
		cmd.Env = append(cmd.Env, sr.metadata.Spec.Env...)
	}

	// Add SBSH-specific env vars
	cmd.Env = append(cmd.Env,
		config.KV(config.SBSH_TERM_SOCKET, sr.metadata.Status.SocketFile),
		config.KV(config.SBSH_TERM_ID, string(sr.metadata.Spec.ID)),
		config.KV(config.SBSH_TERM_NAME, sr.metadata.Spec.Name),
		config.KV(config.SBSH_TERM_PROFILE, sr.metadata.Spec.ProfileName),
		config.KV(config.SB_ROOT_RUN_PATH, sr.metadata.Spec.RunPath),
	)
	// Start the process in its own process group
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

func (sr *Exec) startPty() error {
	// Open a pty
	ptmx, pts, errOpen := pty.Open()

	if errOpen != nil {
		return fmt.Errorf("error opening pty: %w", errOpen)
	}

	sr.ptmx = ptmx
	sr.pts = pts

	sr.cmd.Stdin = sr.pts
	sr.cmd.Stdout = sr.pts
	sr.cmd.Stderr = sr.pts

	errStart := sr.cmd.Start()
	if errStart != nil {
		return fmt.Errorf("error starting command in pty: %w", errStart)
	}

	errC := sr.pts.Close() // pts is now managed by the child process
	if errC != nil {
		sr.logger.Error("error closing pts file descriptor", "err", errC)
		return fmt.Errorf("error closing pts file descriptor: %w", errC)
	}

	// Set initial terminal size to VT100 standard 80x24
	errS := pty.Setsize(sr.ptmx, &pty.Winsize{
		Rows: uint16(vt100Rows),
		Cols: uint16(vt100Cols),
	})
	if errS != nil {
		sr.logger.Error("error setting initial pty size", "err", errS)
		return fmt.Errorf("error setting initial pty size: %w", errS)
	}

	// Set tty device in status metadata
	sr.metadataMu.Lock()
	sr.metadata.Status.Tty = sr.pts.Name()
	sr.metadataMu.Unlock()

	go func() {
		sr.logger.Debug("waiting on child process", "parent_pid", os.Getpid(), "child_pid", sr.cmd.Process.Pid)
		_ = sr.cmd.Wait() // blocks until process exits
		sr.logger.Info("child process has exited", "parent_pid", os.Getpid(), "child_pid", sr.cmd.Process.Pid)
		sr.closeReqCh <- errors.New("the shell process has exited")
	}()

	sr.metadataMu.RLock()
	captureFile := sr.metadata.Spec.CaptureFile
	sr.metadataMu.RUnlock()
	logf, errO := os.OpenFile(captureFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if errO != nil {
		return fmt.Errorf("open log file: %w", errO)
	}

	if errU := sr.updateMetadata(); errU != nil {
		return fmt.Errorf("update metadata: %w", errU)
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

	sr.ptyPipesMu.Lock()
	sr.ptyPipes.pipeInR = pipeInR
	sr.ptyPipes.pipeInW = pipeInW
	sr.ptyPipes.multiOutW = multiOutW
	sr.ptyPipesMu.Unlock()

	go sr.terminalManager(pipeInR, multiOutW)

	return nil
}

func (sr *Exec) terminalManager(pipeInR *os.File, multiOutW io.Writer) {
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
}

func (sr *Exec) terminalManagerReader(multiOutW io.Writer) {
	go func() {
		<-sr.ctx.Done()
		sr.logger.Debug("finishing terminalManagerReader")
		sr.closePTY.Do(func() { _ = sr.ptmx.Close() }) // unblocks Read
		sr.logger.Debug("finished terminalManagerReader")
	}()

	//nolint:mnd // buffer size
	buf := make([]byte, 8192)
	for {
		// READ FROM PTY - WRITE TO PIPE
		n, errRead := sr.ptmx.Read(buf)

		// drain/emit data
		if n > 0 {
			sr.obsMu.Lock()
			sr.lastRead = time.Now()
			sr.bytesOut += uint64(n)
			outputOn := sr.gates.OutputOn
			sr.obsMu.Unlock()

			// Render if output is enabled; otherwise we just drain
			if outputOn {
				sr.logger.Debug("read from pty", "bytes", n)
				sr.logger.Debug("writing to multiOutW")
				_, errWrite := multiOutW.Write(buf[:n])
				sr.logger.Debug("writing to multiOutW done")
				if errWrite != nil {
					sr.logger.Error("error writing raw data to client", "err", errWrite)
					errReturn := fmt.Errorf(
						"terminalManagerReader multiWriter write error: %w: %w",
						ErrPipeWrite,
						errWrite,
					)
					trySendEvent(
						sr.logger,
						sr.evCh,
						Event{ID: sr.id, Type: EvError, Err: errReturn, When: time.Now()},
					)
					return
				}
			}
		}

		// Handle read end/error
		if errRead != nil {
			// treat EOF as info, others as error
			if errRead == io.EOF {
				sr.logger.Info("pty stdout closed (EOF)")
			} else {
				sr.logger.Error("pty stdout error", "err", errRead)
			}
			errReturn := fmt.Errorf("terminalManagerReader pty read error: %w: %w", ErrPipeRead, errRead)
			trySendEvent(
				sr.logger,
				sr.evCh,
				Event{ID: sr.id, Type: EvError, Err: errReturn, When: time.Now()},
			)
			return
		}
	}
}

func (sr *Exec) terminalManagerWriter(pipeInR *os.File) {
	go func() {
		<-sr.ctx.Done()
		sr.logger.Debug("finishing terminalManagerWriter")
		sr.closePTY.Do(func() { _ = sr.ptmx.Close() }) // unblocks Read
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
		n, errRead := pipeInR.Read(buf)
		sr.logger.Debug("read from pipeInR", "bytes", n)
		if n > 0 {
			sr.obsMu.RLock()
			stdinOpen := sr.gates.StdinOpen
			sr.obsMu.RUnlock()
			if stdinOpen {
				sr.logger.Debug("reading from pipeInR (StdinOpen)")
				if _, errWrite := sr.ptmx.Write(buf[:n]); errWrite != nil {
					errReturn := fmt.Errorf(
						"terminalManagerWriter multiWriter write error: %w: %w",
						ErrPipeWrite,
						errWrite,
					)
					trySendEvent(
						sr.logger,
						sr.evCh,
						Event{ID: sr.id, Type: EvError, Err: errReturn, When: time.Now()},
					)
					return
				}
			}
			// else: gate closed, drop input
		}

		if errRead != nil {
			if errRead == io.EOF {
				sr.logger.Info("pipeInR closed (EOF)")
			} else {
				sr.logger.Error("stdin error", "err", errRead)
			}

			errReturn := fmt.Errorf("terminalManagerWriter pty read error: %w: %w", ErrPipeRead, errRead)
			trySendEvent(
				sr.logger,
				sr.evCh,
				Event{ID: sr.id, Type: EvError, Err: errReturn, When: time.Now()},
			)
			return
		}
	}
}
