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
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/internal/env"
)

var closePTY sync.Once

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
		env.KV(env.SES_SOCKET_CTRL, sr.metadata.Status.SocketFile),
		env.KV(env.SES_ID, string(sr.metadata.Spec.ID)),
		env.KV(env.SES_NAME, sr.metadata.Spec.Name),
		env.KV(env.SES_PROFILE, sr.metadata.Spec.ProfileName),
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
		slog.Debug(fmt.Sprintf("[session] pid=%d, waiting on bash pid=%d\r\n", os.Getpid(), sr.cmd.Process.Pid))
		_ = sr.cmd.Wait() // blocks until process exits
		slog.Debug(fmt.Sprintf("[session] pid=%d, bash with pid=%d has exited\r\n", os.Getpid(), sr.cmd.Process.Pid))
		sr.closeReqCh <- errors.New("the shell process has exited")
	}()

	logf, err := os.OpenFile(sr.metadata.Spec.LogFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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
		slog.Debug(fmt.Sprintf("[session] error opening IN pipe: %v\r\n", err))
		return fmt.Errorf("error opening IN pipe: %w", err)
	}

	// StdOut
	// ATTACHED: stream to client (pipeOutW) AND log file
	multiOutW := NewDynamicMultiWriter(logf)

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

	_, _ = sr.Write([]byte("\x15"))
	_, _ = sr.Write([]byte(`export PS1="` + sr.metadata.Spec.Prompt + `"` + "\n")) // ensure

	// we start on a new line
	// sr.Write([]byte(`export PS1="(sbsh-` + sr.id + `) $PS1"` + "\n"))
	// s.pty.Write([]byte("echo 'Hello from Go!'\n"))
	// s.pty.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
	// s.pty.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
	// s.pty.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n")

	return nil
}

func (sr *SessionRunnerExec) terminalManagerReader(multiOutW io.Writer) error {
	go func() {
		<-sr.ctx.Done()
		slog.Debug("[session-runner] finishing terminalManagerReader ")
		closePTY.Do(func() { _ = sr.pty.Close() }) // unblocks Read
		slog.Debug("[session-runner] FINISHED terminalManagerReader ")
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
				//  WRITE TO PIPE
				// PTY writes to pipeOutW
				slog.Debug(fmt.Sprintf("read from pty %q", buf[:n]))
				slog.Debug("[session] writing to multiOutW")
				_, err := multiOutW.Write(buf[:n])
				slog.Debug("[session] writing to multiOutW -> DONE")
				if err != nil {
					slog.Debug("[session] error writing raw data to client")
					return ErrPipeWrite
				}
			}
		}

		// Handle read end/error
		if err != nil {
			slog.Debug(fmt.Sprintf("[session] stdout err  %v:\r\n", err))
			trySendEvent(sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvError, Err: err, When: time.Now()})
			return ErrTerminalRead
		}
	}
}

func (sr *SessionRunnerExec) terminalManagerWriter(pipeInR *os.File) error {
	go func() {
		<-sr.ctx.Done()
		slog.Debug("[session-runner] finishing terminalManagerWriter ")
		closePTY.Do(func() { _ = sr.pty.Close() }) // unblocks Read
		_ = pipeInR.Close()
		slog.Debug("[session-runner] FINISHED terminalManagerWriter ")
	}()

	//nolint:mnd // buffer size
	buf := make([]byte, 4096)
	i := 0
	for {
		// READ FROM PIPE - WRITE TO PTY
		// PTY reads from pipeInR
		slog.Debug(fmt.Sprintf("reading from pipeInR %d\r\n", i)) // quoted, escapes control chars
		i++
		n, err := pipeInR.Read(buf)
		slog.Debug(fmt.Sprintf("read from pipeInR %q", buf[:n])) // quoted, escapes control chars
		if n > 0 {
			if sr.gates.StdinOpen {
				slog.Debug("[session] reading from pipeInR")
				// if _, werr := s.pty.Write(buf[:n]); werr != nil {
				// WRITE TO PIPE
				if _, werr := sr.pty.Write(buf[:n]); werr != nil {
					trySendEvent(sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvError, Err: werr, When: time.Now()})
					return ErrPipeRead
				}
			}
			// else: gate closed, drop input
		}

		if err != nil {
			slog.Debug(fmt.Sprintf("[session] stdin error: %v\r\n", err))
			trySendEvent(sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvError, Err: err, When: time.Now()})
			return ErrTerminalWrite
		}

		// }
	}
}
