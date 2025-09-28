package sessionrunner

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sbsh/pkg/env"
	"syscall"
	"time"

	"github.com/creack/pty"
)

func (sr *SessionRunnerExec) prepareSessionCommand() error {

	// Build the child command with context (so ctx cancel can kill it)
	cmd := exec.CommandContext(sr.ctx, sr.spec.Command, sr.spec.CommandArgs...)
	// Environment: use provided or inherit
	if len(sr.spec.Env) > 0 {
		cmd.Env = sr.spec.Env
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env,
		env.KV(env.SES_SOCKET_CTRL, sr.socketCtrl),
		env.KV(env.SES_SOCKET_IO, sr.socketIO),
		env.KV(env.SES_ID, string(sr.spec.ID)),
		env.KV(env.SES_NAME, sr.spec.Name),
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
		sr.closeReqCh <- fmt.Errorf("the shell process has exited")

	}()

	// Open/prepare a rolling log file (example)
	var logFile string
	if sr.spec.LogDir == "" {
		logFile = filepath.Join(sr.runPath, string(sr.id), "session.log")
	} else {
		logFile = sr.spec.LogDir
	}

	logf, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
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
	// conn reads from pipeOutR
	// PTY writes to pipeOutW
	pipeOutR, pipeOutW, err := os.Pipe()
	if err != nil {
		slog.Debug(fmt.Sprintf("[session] error opening OUT pipe: %v\r\n", err))
		return fmt.Errorf("error opening OUT pipe: %w", err)
	}

	// ATTACHED: stream to client (pipeOutW) AND log file
	multiOutW := io.MultiWriter(pipeOutW, logf)

	sr.ptyPipes.pipeInR = pipeInR
	sr.ptyPipes.pipeInW = pipeInW
	sr.ptyPipes.pipeOutR = pipeOutR
	sr.ptyPipes.pipeOutW = pipeOutW
	sr.ptyPipes.multiOutW = multiOutW

	go sr.terminalManager(pipeInR, multiOutW)

	go sr.handleConnections(pipeInR, pipeInW, pipeOutR, pipeOutW)

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

	sr.Write([]byte(`export PS1="(sbsh-` + sr.id + `) $PS1"` + "\n"))
	// s.pty.Write([]byte("echo 'Hello from Go!'\n"))
	// s.pty.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
	// s.pty.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
	// s.pty.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n")

	return nil
}

func (sr *SessionRunnerExec) terminalManagerReader(multiOutW io.Writer) error {

	go func() {
		<-finishTermMgr
		slog.Debug("[session-runner] finishing terminalManagerReader ")
		// _ = pipeOutW.Close()
		_ = sr.pty.Close() // This unblocks s.pty.Read(...)
		slog.Debug("[session-runner] FINISHED terminalManagerReader ")
	}()

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
				slog.Debug("[session] writing to pipeOutW")
				_, err := multiOutW.Write(buf[:n])
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
		<-finishTermMgr
		slog.Debug("[session-runner] finishing terminalManagerWriter ")
		_ = pipeInR.Close()
		_ = sr.pty.Close() // This unblocks s.pty.Read(...)
		slog.Debug("[session-runner] FINISHED terminalManagerWriter ")
	}()

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
