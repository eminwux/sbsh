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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/rpcclient/session"
)

const (
	// in milliseconds.
	resizeTimeOut = 100
)

func (sr *SupervisorRunnerExec) dialSessionCtrlSocket() error {
	slog.Debug(fmt.Sprintf("[supervisor] %s session on  %d trying to connect to %s\r\n",
		sr.session.Id,
		sr.session.Pid,
		sr.session.SocketFile))

	sr.sessionClient = session.NewUnix(sr.session.SocketFile)
	defer sr.sessionClient.Close()

	//nolint:mnd // timeout duration
	ctx, cancel := context.WithTimeout(sr.ctx, 3*time.Second)
	defer cancel()

	var status api.SessionStatusMessage
	if err := sr.sessionClient.Status(ctx, &status); err != nil {
		return fmt.Errorf("status failed: %w", err)
	}

	slog.Debug(fmt.Sprintf("[supervisor] rpc->session (Status): %+v\r\n", status))

	return nil
}

func (sr *SupervisorRunnerExec) attachIOSocket() error {
	// Connected, now we enable raw mode
	if err := sr.toBashUIMode(); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] initial raw mode failed: %v", err))
	}

	// We want half-closes; UnixConn exposes CloseRead/CloseWrite
	uc, _ := sr.ioConn.(*net.UnixConn)

	//nolint:mnd // channel buffer size
	errCh := make(chan error, 2)

	// WRITER stdin -> socket
	go func() {
		//nolint:mnd // buffer size
		buf := make([]byte, 32*1024) // 32 KiB buffer, like io.Copy
		var total int64
		var e error

		for {
			slog.Debug("stdin->socket pre-read")
			n, rerr := os.Stdin.Read(buf)
			slog.Debug(fmt.Sprintf("stdin->socket post-read: %d", n))

			if n > 0 {
				written := 0
				for written < n {
					slog.Debug("stdin->socket pre-write")
					m, werr := sr.ioConn.Write(buf[written:n])
					slog.Debug(fmt.Sprintf("stdin->socket post-write: %d", m))
					if werr != nil {
						e = werr
						goto done
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				e = rerr
				break
			}
		}

	done:
		// tell peer we're done sending (but still willing to read)
		if uc != nil {
			_ = uc.CloseWrite()
		}

		// send event (EOF or error while copying stdin -> socket)
		if errors.Is(e, io.EOF) {
			slog.Debug("[supervisor] stdin reached EOF\r\n")
			trySendEvent(sr.events, SupervisorRunnerEvent{
				ID:   sr.session.Id,
				Type: EvCmdExited,
				Err:  e,
				When: time.Now(),
			})
		} else if e != nil {
			slog.Debug(fmt.Sprintf("[supervisor] stdin->socket error: %v\r\n", e))
			trySendEvent(sr.events, SupervisorRunnerEvent{
				ID:   sr.session.Id,
				Type: EvError,
				Err:  e,
				When: time.Now(),
			})
		}

		errCh <- e
	}()

	// READER socket -> stdout
	go func() {
		//nolint:mnd // buffer size
		buf := make([]byte, 32*1024) // 32 KiB buffer like io.Copy
		var total int64
		var e error

		for {
			slog.Debug("socket->stdout pre-read")
			n, rerr := sr.ioConn.Read(buf)
			slog.Debug(fmt.Sprintf("socket->stdout post-read: %d", n))

			if n > 0 {
				written := 0
				for written < n {
					slog.Debug("socket->stdout pre-write")
					m, werr := os.Stdout.Write(buf[written:n])
					slog.Debug(fmt.Sprintf("socket->stdout post-write: %d", m))
					if werr != nil {
						e = werr
						goto done
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				e = rerr
				break
			}
		}

	done:
		// we won't read further; let the other goroutine finish
		if uc != nil {
			_ = uc.CloseRead()
		}

		// send event (EOF or error while copying socket -> stdout)
		if errors.Is(e, io.EOF) {
			slog.Debug("[supervisor] socket closed (EOF)\r\n")
			trySendEvent(sr.events, SupervisorRunnerEvent{
				ID:   sr.session.Id,
				Type: EvCmdExited,
				Err:  e,
				When: time.Now(),
			})
		} else if e != nil {
			slog.Debug(fmt.Sprintf("[supervisor] socket->stdout error: %v\r\n", e))
			trySendEvent(sr.events, SupervisorRunnerEvent{
				ID:   sr.session.Id,
				Type: EvError,
				Err:  e,
				When: time.Now(),
			})
		}

		errCh <- e
	}()

	// Force resize
	syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)

	go func() error {
		// Wait for either context cancel or one side finishing
		select {
		case <-sr.ctx.Done():
			slog.Debug("[supervisor-runner] context done\r\n")
			_ = sr.ioConn.Close() // unblock goroutines
			<-errCh
			<-errCh
			return sr.ctx.Err()
		case e := <-errCh:
			// one direction ended; close and wait for the other
			_ = sr.ioConn.Close()
			<-errCh
			// treat EOF as normal detach
			if errors.Is(e, io.EOF) || e == nil {
				return nil
			}
			return e
		}
	}()

	return nil
}

func (sr *SupervisorRunnerExec) attach() error {
	var response struct{}
	var conn net.Conn
	var err error
	conn, err = sr.sessionClient.Attach(sr.ctx, &sr.id, &response)
	if err != nil {
		return err
	}
	slog.Debug("[supervisor] Received connection")

	sr.ioConn = conn

	return nil
}

func (sr *SupervisorRunnerExec) forwardResize() error {
	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, err := pty.Getsize(os.Stdin); err == nil {
		//nolint:mnd // timeout duration
		ctx, cancel := context.WithTimeout(sr.ctx, 100*time.Millisecond)
		defer cancel()

		if err := sr.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: int(cols), Rows: int(rows)}); err != nil {
			return fmt.Errorf("status failed: %w", err)
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-sr.ctx.Done():
				signal.Stop(ch)
				defer close(ch)
				return
			case <-ch:
				// slog.Debug("[supervisor] window change\r\n")
				// Query current terminal size again on every WINCH
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil {
					// harmless: keep going; terminal may be detached briefly
					continue
				}
				ctx, cancel := context.WithTimeout(sr.ctx, resizeTimeOut*time.Millisecond)
				defer cancel()
				if err := sr.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: int(cols), Rows: int(rows)}); err != nil {
					// Don't kill the process on resize failure; just log
					slog.Debug(fmt.Sprintf("resize RPC failed: %v\r\n", err))
				}
			}
		}
	}()
	return nil
}
