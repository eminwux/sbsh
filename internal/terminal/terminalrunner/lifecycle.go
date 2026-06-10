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
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/pkg/api"
)

const (
	deleteTerminalsDir bool = false
	waitReadyPeriod         = 10 * time.Millisecond
	// reapAfterKillGrace bounds how long shutdownChild waits for the tracked
	// child to actually be reaped after the SIGKILL escalation. SIGKILL only
	// queues the signal; the kernel still has to schedule the child, turn it
	// into a zombie, and the harvester (the PID-1 reaper in init mode, or
	// watchChildExit's cmd.Wait otherwise) still has to reap it. Close tears
	// the reaper down via reaper.Stop right after shutdownChild returns, so
	// without this wait the reaper's final drain races the kill-to-zombie
	// window: a late zombie is never reaped and watchChildExit parks forever on
	// TrackedExitCh. Bounded so an unkillable (D-state) child cannot wedge
	// Close. See #398.
	reapAfterKillGrace = 5 * time.Second
)

func (sr *Exec) StartTerminal(evCh chan<- Event) error {
	// 1) Update state to Initializing
	sr.logger.Info("starting terminal", "id", sr.id)
	if err := sr.updateTerminalState(api.Initializing); err != nil {
		sr.logger.Error("failed to update terminal state on initializing", "id", sr.id, "err", err)
	}
	sr.evCh = evCh

	// 2) Tighten/relax the per-terminal log file's permissions per the
	//    spec. The file was created by SetupFileLogger before the runner
	//    started, so it already exists at 0o600; this re-chmods (and
	//    optionally chowns the group) to match the resolved mode/gid.
	//    Skip silently when the spec did not name a log file — some test
	//    fakes wire a stderr-only logger and leave the path empty.
	if err := sr.applyLogFilePerms(); err != nil {
		sr.logger.Error("failed to apply log file perms", "id", sr.id, "err", err)
		return fmt.Errorf("failed to apply log file perms for terminal %s: %w", sr.id, err)
	}

	// 3) Prepare terminal command
	if err := sr.prepareTerminalCommand(); err != nil {
		sr.logger.Error("failed to run terminal command", "id", sr.id, "err", err)
		return fmt.Errorf("failed to run terminal command for terminal %s: %w", sr.id, err)
	}

	// 4) Start PTY
	if err := sr.startPty(); err != nil {
		sr.logger.Error("failed to start PTY", "id", sr.id, "err", err)
		return fmt.Errorf("failed to start PTY for terminal %s: %w", sr.id, err)
	}

	// 5) Update state to Starting
	if err := sr.updateTerminalState(api.Starting); err != nil {
		sr.logger.Error("failed to update terminal state on starting", "id", sr.id, "err", err)
		return fmt.Errorf("failed to update terminal state on starting: %w", err)
	}

	// 6) Wait for terminal to close in background
	sr.logger.Info("PTY started", "id", sr.id)
	go sr.waitOnTerminal()

	return nil
}

// applyLogFilePerms re-chmods (and optionally chowns the group of) the
// per-terminal log file. The file is created by SetupFileLogger in the
// parent process before the runner starts, so by the time we reach this
// step it already exists at 0o600. The chmod/chown is by path because
// the runner does not own the file handle.
//
// An empty Spec.LogFile is treated as "no log file configured" and the
// step is a no-op — this keeps test runners that use an io.Discard
// logger from tripping a missing-file error.
func (sr *Exec) applyLogFilePerms() error {
	sr.metadataMu.RLock()
	logFile := sr.metadata.Spec.LogFile
	mode := sr.metadata.Spec.LogFileMode
	var gid *int
	if g := sr.metadata.Spec.LogFileGID; g != nil {
		gv := *g
		gid = &gv
	}
	sr.metadataMu.RUnlock()

	if logFile == "" {
		return nil
	}
	return sr.applyArtifactPerms("log-file", logFile, mode, gid)
}

func (sr *Exec) waitOnTerminal() {
	err := <-sr.closeReqCh
	sr.logger.Info("terminal close requested", "id", sr.id, "err", err)
	sr.logger.Debug("sending EvCmdExited event", "id", sr.id)
	trySendEvent(sr.logger, sr.evCh, Event{ID: sr.id, Type: EvCmdExited, Err: err, When: time.Now()})
}

// watchChildExit waits for the tracked child to exit and publishes the
// "process exited" close request. When PID-1 init mode is on, the reaper has
// already consumed the SIGCHLD, so os/exec.Cmd.Wait would race and may see
// ECHILD instead of the real status. The reaper's TrackedExitCh is the
// authoritative signal in that case; outside init mode, cmd.Wait() is.
func (sr *Exec) watchChildExit() {
	pid := 0
	if sr.cmd != nil && sr.cmd.Process != nil {
		pid = sr.cmd.Process.Pid
	}
	sr.logger.Debug("watching child process", "parent_pid", os.Getpid(), "child_pid", pid)

	var exitErr error
	if sr.initMode && sr.reaper != nil {
		select {
		case code, ok := <-sr.reaper.TrackedExitCh():
			if ok {
				exitErr = fmt.Errorf("shell process exited: code=%d", code)
			} else {
				exitErr = errors.New("shell process exited")
			}
		case <-sr.ctx.Done():
			// Close cancelled the context before the reaper delivered the
			// tracked exit — e.g. the child became a zombie only after the
			// post-SIGKILL reap window (reapAfterKillGrace) elapsed. Escape
			// rather than park forever on TrackedExitCh; the closeReqCh send
			// below is itself ctx-guarded, so this goroutine always
			// terminates regardless. See #398.
			exitErr = errors.New("shell process exited (close cancelled tracked-exit wait)")
		}
		// Release the os.Process handle so Go doesn't hold on to the
		// kernel reference now that the reaper has already reaped the
		// PID. A subsequent cmd.Wait() would return ECHILD — we skip it.
		// Take runtimeMu around the Release: it mutates the shared
		// *os.Process, and shutdownChild reads cmd.Process.Pid under the
		// same lock, so without it the two race once Close's post-SIGKILL
		// wait lets this goroutine proceed. See #398.
		sr.runtimeMu.Lock()
		if sr.cmd != nil && sr.cmd.Process != nil {
			_ = sr.cmd.Process.Release()
		}
		sr.runtimeMu.Unlock()
	} else {
		// cmd.Wait()'s *exec.ExitError carries the real exit code (and signal
		// info on signaled deaths) — propagate it so EvCmdExited consumers can
		// distinguish "shell exited 0" from "shell killed by SIGSEGV".
		if werr := sr.cmd.Wait(); werr != nil {
			exitErr = fmt.Errorf("shell process exited: %w", werr)
		} else {
			exitErr = errors.New("shell process exited (code 0)")
		}
	}

	sr.logger.Info("child process has exited", "parent_pid", os.Getpid(), "child_pid", pid)
	sr.markChildDone()
	// waitOnTerminal reads from closeReqCh; send is allowed to block
	// briefly if Close is racing.
	select {
	case sr.closeReqCh <- exitErr:
	case <-sr.ctx.Done():
	}
}

// shutdownChild runs the graceful shutdown sequence for the tracked child:
//
//  1. If the child has already exited, return immediately.
//  2. Send SIGTERM to the child's process group.
//  3. Wait up to grace for child exit.
//  4. If still alive, send SIGKILL to the process group.
//  5. After SIGKILL, wait (bounded by reapAfterKillGrace) for the child to be
//     reaped so Close does not tear the reaper down before the zombie forms.
//
// Grace <= 0 falls back to the package default. Must only be called after
// startPty has run (cmd and cmd.Process are set).
func (sr *Exec) shutdownChild(grace time.Duration) {
	// Snapshot cmd and the child's pid under runtimeMu. startPty publishes cmd
	// (and its Process) under the same lock, so this is otherwise a data race
	// against a Stop-during-startup Close (#396). Reading cmd.Process.Pid under
	// the lock also serializes it against watchChildExit's cmd.Process.Release
	// (also taken under runtimeMu): once the post-SIGKILL reap wait below lets
	// watchChildExit run to completion, an unlocked pid read here would race
	// that Release write on the shared *os.Process. See #398.
	sr.runtimeMu.Lock()
	cmd := sr.cmd
	var pid int
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	sr.runtimeMu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if sr.childDone() {
		return
	}
	if grace <= 0 {
		grace = profile.DefaultShutdownGrace
	}

	// Setsid: true ensures pgid == pid for the child.
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		sr.logger.Warn("could not SIGTERM child process group", "id", sr.id, "pid", pid, "err", err)
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-sr.childDoneCh:
		sr.logger.Debug("child exited within grace period", "id", sr.id, "grace", grace)
		return
	case <-timer.C:
		sr.logger.Warn("child did not exit within grace, escalating to SIGKILL",
			"id", sr.id, "pid", pid, "grace", grace)
	}

	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		sr.logger.Warn("could not SIGKILL child process group", "id", sr.id, "pid", pid, "err", err)
	}

	// SIGKILL is asynchronous: it only queues the signal. The child still has
	// to be scheduled, die, transition to a zombie, and be harvested — by the
	// PID-1 reaper in init mode, or by watchChildExit's cmd.Wait otherwise —
	// before childDoneCh closes. Block (bounded) until that happens so Close
	// does not call reaper.Stop while the zombie has not formed yet:
	// reaper.Stop's final drain would otherwise miss it and the child would
	// leak as a permanent zombie with watchChildExit parked on TrackedExitCh.
	// See #398.
	killTimer := time.NewTimer(reapAfterKillGrace)
	defer killTimer.Stop()
	select {
	case <-sr.childDoneCh:
		sr.logger.Debug("child reaped after SIGKILL escalation", "id", sr.id, "pid", pid)
	case <-killTimer.C:
		sr.logger.Warn("child not reaped within grace after SIGKILL; reaper teardown may leak a zombie",
			"id", sr.id, "pid", pid, "grace", reapAfterKillGrace)
	}
}

func (sr *Exec) Close(reason error) error {
	sr.logger.Info("closing terminal", "id", sr.id, "reason", reason)

	if err := sr.updateTerminalState(api.Exited); err != nil {
		// The permission-denied case is already surfaced once, actionably, by
		// updateTerminalState -> noteMetadataWriteErr; demote here so close
		// does not pair a redundant ERROR with that single WARN. See #345.
		if errors.Is(err, fs.ErrPermission) {
			sr.logger.Debug("terminal state not persisted on close (dir not writable)", "id", sr.id, "err", err)
		} else {
			sr.logger.Error("failed to update terminal state", "id", sr.id, "err", err)
		}
	}

	// Latch `closing` before the graceful child shutdown so a startPty
	// racing this Close (Stop RPC during startup) observes it under
	// runtimeMu and aborts before cmd.Start. This closes the orphaned-child
	// hole: a child forked after shutdownChild's early return (cmd.Process
	// still nil) would never be signalled, since cmd.Cancel is a deliberate
	// no-op and Close has already passed its only kill site. See #396.
	sr.runtimeMu.Lock()
	sr.closing = true
	sr.runtimeMu.Unlock()

	// Graceful shutdown of the child before tearing anything else down:
	// SIGTERM -> wait up to ShutdownGrace -> SIGKILL. This runs regardless
	// of PID-1 status; removing the always-hard-kill default is correct in
	// every mode.
	sr.shutdownChild(sr.metadata.Spec.ShutdownGrace)

	sr.ctxCancel()

	sr.logger.Debug("terminal closed", "id", sr.id)

	// Tear down PID-1 helpers (no-ops outside init mode). Read/clear under
	// runtimeMu: startPty publishes stopSignalForwarder concurrently. See #396.
	sr.runtimeMu.Lock()
	stopForwarder := sr.stopSignalForwarder
	sr.stopSignalForwarder = nil
	sr.runtimeMu.Unlock()
	if stopForwarder != nil {
		stopForwarder()
	}
	if sr.reaper != nil {
		sr.reaper.Stop()
	}

	// stop accepting
	if sr.lnCtrl != nil {
		if err := sr.lnCtrl.Close(); err != nil {
			sr.logger.Warn("could not close IO listener", "id", sr.id, "err", err)
		}
	}

	// close clients
	//
	// Snapshot multiOutW outside clientsMu: cleanupClient takes
	// ptyPipesMu then clientsMu (via removeClient), so acquiring them in
	// the opposite order here would invert the lock graph.
	//
	// Per-client teardown mirrors cleanupClient (connections.go) and the
	// Detach RPC path: drop the attacher's bounded writer from the
	// fan-out, Close it to wake its drain goroutine, then close the conn.
	// Pre-fix Close only closed conn and relied on the dualcopier
	// reader-side onClose firing detach to indirectly drain the
	// goroutine; that path still runs (idempotently, gated by
	// detachOnce + clear() below), but the indirect dependency is no
	// longer load-bearing for goroutine teardown. See #230.
	//
	// clear() instead of `sr.clients = nil`: a concurrent addClient call
	// can race past Close's unlock (per-conn RPC handlers outlive the
	// accept loop's ctx-cancel exit), and writing to a nil map panics.
	// Emptying the map keeps it addressable so the late write is a benign
	// stale-entry no-op shape. See #225.
	sr.ptyPipesMu.RLock()
	multiOutW := sr.ptyPipes.multiOutW
	sr.ptyPipesMu.RUnlock()

	sr.clientsMu.Lock()
	for _, c := range sr.clients {
		if c.outWriter != nil {
			if multiOutW != nil {
				multiOutW.Remove(c.outWriter)
			}
			_ = c.outWriter.Close()
		}
		if err := c.conn.Close(); err != nil {
			sr.logger.Warn("could not close client connection", "id", sr.id, "client", c.id, "err", err)
		}
	}
	clear(sr.clients)
	sr.clientsMu.Unlock()

	// close all output subscribers
	sr.closeAllSubscribers()

	// Snapshot ptmx under runtimeMu: startPty publishes it concurrently. The
	// closePTY sync.Once still serializes the actual close against the
	// terminalManager goroutines' own ctx-cancel close. See #396.
	sr.runtimeMu.Lock()
	ptmx := sr.ptmx
	sr.runtimeMu.Unlock()
	if ptmx != nil {
		var err error
		sr.closePTY.Do(func() {
			err = ptmx.Close()
		})
		if err != nil {
			sr.logger.Warn("could not close pty", "id", sr.id, "err", err)
		}
	}

	// Close the capture live-segment fd. Sequenced after closeAllSubscribers
	// and ptmx teardown so the PTY reader has stopped writing to multiOutW
	// (which holds the rotating capture writer as its always-on sink).
	// closeCapture guards against double-Close per #229's idempotency AC.
	// Snapshot capture under runtimeMu: startPty publishes it concurrently.
	// closeCapture guards against double-Close per #229's idempotency AC.
	sr.runtimeMu.Lock()
	capture := sr.capture
	sr.runtimeMu.Unlock()
	if capture != nil && sr.closeCapture != nil {
		sr.closeCapture.Do(func() {
			if err := capture.Close(); err != nil {
				sr.logger.Warn("could not close capture file", "id", sr.id, "err", err)
			}
		})
	}

	// remove Ctrl socket
	sr.metadataMu.RLock()
	socketFile := sr.metadata.Status.SocketFile
	terminalRunPath := sr.metadata.Status.TerminalRunPath
	sr.metadataMu.RUnlock()
	if err := os.Remove(socketFile); err != nil {
		sr.logger.Warn("couldn't remove Ctrl socket", "id", sr.id, "socket", socketFile, "err", err)
	}

	if deleteTerminalsDir {
		if err := os.RemoveAll(filepath.Dir(terminalRunPath)); err != nil {
			sr.logger.Warn(
				"couldn't remove terminal directory",
				"id",
				sr.id,
				"dir",
				terminalRunPath,
				"err",
				err,
			)
		}
	}

	// Guard close(closedCh) so a repeat Close on the same runner does not
	// panic on an already-closed channel. The other once-only steps in this
	// function (closePTY, closeCapture, childDoneOnce) follow the same
	// per-resource sync.Once pattern. See #242.
	sr.closeClosedCh.Do(func() {
		close(sr.closedCh)
	})
	return nil
}

func (sr *Exec) Resize(args api.ResizeArgs) {
	_ = pty.Setsize(sr.ptmx, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
	// Keep the screen model's grid in step with the PTY winsize so a
	// Screenshot reflects the dimensions the child program is drawing for.
	sr.ptyPipesMu.RLock()
	screen := sr.ptyPipes.screen
	sr.ptyPipesMu.RUnlock()
	if screen != nil {
		screen.resize(args.Cols, args.Rows)
	}
}

// Screenshot returns a decoded snapshot of the terminal's current screen
// from the warm-fed vt-parser model. It reads the live grid, not the raw
// capture bytes.
func (sr *Exec) Screenshot(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
	sr.ptyPipesMu.RLock()
	screen := sr.ptyPipes.screen
	sr.ptyPipesMu.RUnlock()
	if screen == nil {
		return nil, errors.New("terminal not running")
	}
	return screen.snapshot(), nil
}

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

func (sr *Exec) Attach(req *api.AttachRequest, response *api.ResponseWithFD) error {
	// Reject an Attach that lands in the window between the control RPC
	// server becoming ready and startPty publishing ptyPipes. multiOutW
	// (and pipeInW) are published together at terminal.go's
	// ptyPipes.multiOutW assignment, so a nil multiOutW means the PTY
	// fan-out is not wired yet. Without this guard handleClient would call
	// multiOutW.Add on a nil *DynamicMultiWriter and panic on a bare
	// goroutine with no recover, killing the daemon. Failing here surfaces
	// a clean RPC error to the caller and avoids allocating the socketpair
	// at all. Mirrors the sibling Subscribe guard (subscribe.go). See #395.
	sr.ptyPipesMu.RLock()
	multiOutW := sr.ptyPipes.multiOutW
	sr.ptyPipesMu.RUnlock()
	if multiOutW == nil {
		return fmt.Errorf("Attach: terminal not running")
	}

	cliFD, err := sr.CreateNewClient(&req.ClientID, req.FullCapture)
	if err != nil {
		return err
	}

	payload := struct {
		OK bool `json:"ok"`
	}{OK: true}

	fds := []int{cliFD}

	sr.logger.Debug("Attach response", "id", req.ClientID, "fullCapture", req.FullCapture, "ok", payload.OK, "fds", fds)

	response.JSON = payload
	response.FDs = fds

	return nil
}

func (sr *Exec) Detach(id *api.ID) error {
	// 1) Lookup
	ioClient, okClient := sr.getClient(*id)
	if !okClient {
		return fmt.Errorf("client %s not found among clients", *id)
	}

	// 2) Remove the bounded writer from the fan-out first so no more PTY
	// bytes target this attacher's ring.
	sr.ptyPipesMu.RLock()
	multiOutW := sr.ptyPipes.multiOutW
	sr.ptyPipesMu.RUnlock()

	// Snapshot outWriter under clientsMu — handleClient publishes it under
	// the same lock, so a Detach arriving immediately after Attach (before
	// handleClient finished wiring the writer) would otherwise race the
	// field write (issue #355).
	sr.clientsMu.RLock()
	outWriter := ioClient.outWriter
	sr.clientsMu.RUnlock()

	if outWriter != nil {
		multiOutW.Remove(outWriter)
		// Close signals the drain goroutine to flush any pending bytes
		// and exit. The drain's deferred conn.Close releases the
		// socketpair fd. The async block below force-closes the conn
		// after a short grace so a stuck drain can't pin the fd.
		_ = outWriter.Close()
	}

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

	if err := sr.updateTerminalAttachers(); err != nil {
		sr.logger.Warn("failed to update metadata on client cleanup", "err", err)
	}

	return nil
}

func (sr *Exec) SetupShell() error {
	if err := sr.updateTerminalState(api.SettingUp); err != nil {
		sr.logger.Error("failed to update terminal state on setting up", "id", sr.id, "err", err)
		return fmt.Errorf("failed to update terminal state on setting up: %w", err)
	}

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
	// Take metadataMu around the Spec.Prompt read-modify-write: an attaching
	// client polls the State RPC (which copies sr.metadata under metadataMu)
	// every ~50ms during setup, and an RLock'd reader does not exclude a
	// lock-free writer. Snapshot the resolved prompt under the lock and write
	// it to the PTY outside the critical section. See #388.
	sr.metadataMu.Lock()
	setPrompt := sr.metadata.Spec.SetPrompt
	if setPrompt && sr.metadata.Spec.Prompt == "" {
		sr.metadata.Spec.Prompt = "(sbsh-" + string(sr.metadata.Spec.ID) + ")"
	}
	prompt := sr.metadata.Spec.Prompt
	sr.metadataMu.Unlock()

	if setPrompt {
		promptCmd := `export PS1=` + prompt + "\n"

		sr.logger.Debug("setupShell: setting prompt", "cmd", promptCmd)
		if _, err := sr.Write([]byte(promptCmd)); err != nil {
			sr.logger.Error("failed to set prompt", "id", sr.id, "cmd", promptCmd, "err", err)
			return fmt.Errorf("failed to set prompt: %w", err)
		}
	}

	//nolint:mnd // small delay between commands
	time.Sleep(10 * time.Millisecond)

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
	sr.logger.Info("PostAttachShell: waiting for terminal to be ready", "id", sr.id)

	for sr.currentState() != api.Ready {
		select {
		case <-sr.ctx.Done():
			sr.logger.Error("PostAttachShell: context done", "id", sr.id)
			return sr.ctx.Err()
		case <-time.After(waitReadyPeriod):
			// loop again until ready
		}
	}

	sr.logger.Info("PostAttachShell: updating terminal state to post attach", "id", sr.id)

	// Attach-time metadata writes are best-effort: metadata.json is an
	// observability/reconnect artifact, not the source of truth for the
	// live PTY, and aborting attach on a persistence failure (ENOSPC,
	// perm-denied) leaves a healthy terminal unreachable. The error is
	// already actionably logged by noteMetadataWriteErr; demote the
	// return so the attach proceeds. updateTerminalState's write-first
	// commit guarantees no strand on in-memory state. See #345, #373.
	if err := sr.updateTerminalState(api.PostAttach); err != nil {
		sr.logger.Warn("PostAttachShell: state write failed, continuing", "id", sr.id, "err", err)
	}

	// Snapshot the PostAttach stage slice under the read lock: SetupShell
	// writes Spec.Prompt under metadataMu, so Spec is not immutable and a
	// lock-free read here would not be excluded by that writer. See #388.
	sr.metadataMu.RLock()
	postAttachStages := sr.metadata.Spec.Stages.PostAttach
	sr.metadataMu.RUnlock()
	if err := sr.writeStage(&postAttachStages); err != nil {
		return err
	}

	if err := sr.updateTerminalState(api.Ready); err != nil {
		sr.logger.Warn("PostAttachShell: state write failed, continuing", "id", sr.id, "err", err)
	}
	return nil
}

func (sr *Exec) OnInitShell() error {
	if err := sr.updateTerminalState(api.OnInit); err != nil {
		sr.logger.Error("failed to update terminal state on on init", "id", sr.id, "err", err)
		return fmt.Errorf("failed to update terminal state on on init: %w", err)
	}

	if err := sr.writeStage(&sr.metadata.Spec.Stages.OnInit); err != nil {
		return err
	}

	if err := sr.updateTerminalState(api.Ready); err != nil {
		sr.logger.Error("failed to update terminal state on ready", "id", sr.id, "err", err)
	}
	return nil
}
