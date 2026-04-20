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

package spawn

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// process wraps an exec.Cmd launched in a detached (Setsid) process
// group and tracks its lifecycle through a single background reaper
// goroutine. It is the shared plumbing underneath TerminalHandle and
// ClientHandle.
type process struct {
	cmd    *exec.Cmd
	logger *slog.Logger

	exitOnce sync.Once
	exitCh   chan struct{}
	exitMu   sync.Mutex
	exitErr  error
}

// startProcess runs cmd detached and spins a reaper goroutine that
// records the exit error and closes exitCh so WaitClose callers unblock.
func startProcess(cmd *exec.Cmd, logger *slog.Logger) (*process, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	p := &process{
		cmd:    cmd,
		logger: logger,
		exitCh: make(chan struct{}),
	}

	go func() {
		waitErr := cmd.Wait()
		p.exitMu.Lock()
		p.exitErr = waitErr
		p.exitMu.Unlock()
		p.exitOnce.Do(func() { close(p.exitCh) })
		if logger != nil {
			if waitErr != nil {
				logger.Debug("spawned process exited", "pid", cmd.Process.Pid, "error", waitErr)
			} else {
				logger.Debug("spawned process exited", "pid", cmd.Process.Pid)
			}
		}
	}()

	return p, nil
}

// pid returns the OS PID of the spawned process.
func (p *process) pid() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// exited reports whether the reaper has observed process exit.
func (p *process) exited() bool {
	select {
	case <-p.exitCh:
		return true
	default:
		return false
	}
}

// takeExitErr returns the recorded exit error (safe after exitCh closes).
func (p *process) takeExitErr() error {
	p.exitMu.Lock()
	defer p.exitMu.Unlock()
	return p.exitErr
}

// waitExit blocks until the process exits or ctx is done. It returns
// the process exit error on exit, ctx.Err() on cancellation.
func (p *process) waitExit(ctx context.Context) error {
	select {
	case <-p.exitCh:
		return p.takeExitErr()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// signal sends sig to the spawned process. It is a no-op if the
// process has already exited.
func (p *process) signal(sig os.Signal) error {
	if p.exited() {
		return nil
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return errors.New("spawn: process not started")
	}
	return p.cmd.Process.Signal(sig)
}

// waitWithTimeout blocks until exit, ctx done, or d elapses. It
// distinguishes the three via the returned error:
//   - nil: process exited (consult takeExitErr for the real error)
//   - ctx.Err(): ctx was canceled/expired
//   - context.DeadlineExceeded: local step timeout elapsed (ctx still alive)
func (p *process) waitWithTimeout(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return p.waitExit(ctx)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.exitCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return context.DeadlineExceeded
	}
}

// gracefulShutdown attempts a clean stop: run preSignal (e.g. a Stop
// RPC) if provided, wait gracePeriod for natural exit, then SIGTERM +
// wait, then SIGKILL. Returns the process exit error (or ctx.Err() if
// the caller's context was canceled).
func (p *process) gracefulShutdown(
	ctx context.Context,
	gracePeriod time.Duration,
	preSignal func(context.Context) error,
) error {
	if p.exited() {
		return p.takeExitErr()
	}
	if preSignal != nil {
		if err := preSignal(ctx); err != nil && p.logger != nil {
			p.logger.DebugContext(ctx, "graceful pre-signal hook failed", "error", err)
		}
	}
	if gracePeriod <= 0 {
		gracePeriod = defaultGracePeriod
	}

	if err := p.waitWithTimeout(ctx, gracePeriod); err == nil {
		return p.takeExitErr()
	} else if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := p.signal(syscall.SIGTERM); err != nil && p.logger != nil {
		p.logger.DebugContext(ctx, "SIGTERM failed", "error", err)
	}
	if err := p.waitWithTimeout(ctx, gracePeriod); err == nil {
		return p.takeExitErr()
	} else if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := p.signal(syscall.SIGKILL); err != nil && p.logger != nil {
		p.logger.DebugContext(ctx, "SIGKILL failed", "error", err)
	}
	return p.waitExit(ctx)
}

const defaultGracePeriod = 2 * time.Second

// resolveStdio wires caller-supplied stdio onto cmd. Anything left nil
// is redirected to /dev/null — the child must never inherit the
// parent's TTY unless the caller explicitly asked for it. Returned
// files should be closed by the caller after cmd.Start() returns; they
// are duplicated into the child at that point.
func resolveStdio(cmd *exec.Cmd, stdin io.Reader, stdout, stderr io.Writer) ([]*os.File, error) {
	var opened []*os.File

	openNull := func(flag int) (*os.File, error) {
		f, err := os.OpenFile(os.DevNull, flag, 0)
		if err != nil {
			return nil, err
		}
		opened = append(opened, f)
		return f, nil
	}

	if stdin == nil {
		f, err := openNull(os.O_RDONLY)
		if err != nil {
			return opened, err
		}
		cmd.Stdin = f
	} else {
		cmd.Stdin = stdin
	}

	if stdout == nil {
		f, err := openNull(os.O_WRONLY)
		if err != nil {
			return opened, err
		}
		cmd.Stdout = f
	} else {
		cmd.Stdout = stdout
	}

	if stderr == nil {
		f, err := openNull(os.O_WRONLY)
		if err != nil {
			return opened, err
		}
		cmd.Stderr = f
	} else {
		cmd.Stderr = stderr
	}

	return opened, nil
}

// closeFiles closes every file, ignoring errors. Used to drop the
// /dev/null handles after cmd.Start() has duplicated them.
func closeFiles(files []*os.File) {
	for _, f := range files {
		_ = f.Close()
	}
}
