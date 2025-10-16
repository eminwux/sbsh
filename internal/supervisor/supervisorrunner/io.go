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
	sr.logger.DebugContext(sr.ctx, "dialSessionCtrlSocket: connecting to session",
		"session_id", sr.session.Id,
		"pid", sr.session.Pid,
		"socket_file", sr.session.SocketFile)

	sr.sessionClient = session.NewUnix(sr.session.SocketFile, sr.logger)
	defer sr.sessionClient.Close()

	ctx, cancel := context.WithTimeout(sr.ctx, 3*time.Second)
	defer cancel()

	ping := api.PingMessage{Message: "PING"}
	var pong api.PingMessage
	if err := sr.sessionClient.Ping(ctx, &ping, &pong); err != nil {
		sr.logger.ErrorContext(sr.ctx, "dialSessionCtrlSocket: ping failed", "error", err)
		return fmt.Errorf("ping failed: %w", err)
	}

	sr.logger.InfoContext(sr.ctx, "dialSessionCtrlSocket: session ping successful", "response", pong.Message)
	return nil
}

func (sr *SupervisorRunnerExec) attachIOSocket() error {
	// Connected, now we enable raw mode
	if err := sr.toBashUIMode(); err != nil {
		sr.logger.ErrorContext(sr.ctx, "attachIOSocket: initial raw mode failed", "error", err)
	}

	// We want half-closes; UnixConn exposes CloseRead/CloseWrite
	uc, _ := sr.ioConn.(*net.UnixConn)

	errCh := make(chan error, 2)

	// WRITER stdin -> socket
	go func() {
		buf := make([]byte, 32*1024) // 32 KiB buffer, like io.Copy
		var total int64
		var e error

		for {
			sr.logger.DebugContext(sr.ctx, "stdin->socket pre-read")
			n, rerr := os.Stdin.Read(buf)
			sr.logger.DebugContext(sr.ctx, "stdin->socket post-read", "n", n)

			if n > 0 {
				written := 0
				for written < n {
					sr.logger.DebugContext(sr.ctx, "stdin->socket pre-write")
					m, werr := sr.ioConn.Write(buf[written:n])
					sr.logger.DebugContext(sr.ctx, "stdin->socket post-write", "m", m)
					if werr != nil {
						sr.logger.ErrorContext(sr.ctx, "stdin->socket write error", "error", werr)
						e = werr
						goto done
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				sr.logger.ErrorContext(sr.ctx, "stdin->socket read error", "error", rerr)
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
			sr.logger.InfoContext(sr.ctx, "stdin reached EOF")
			trySendEvent(sr.logger, sr.events, SupervisorRunnerEvent{
				ID:   sr.session.Id,
				Type: EvCmdExited,
				Err:  e,
				When: time.Now(),
			})
		} else if e != nil {
			sr.logger.ErrorContext(sr.ctx, "stdin->socket error", "error", e)
			trySendEvent(sr.logger, sr.events, SupervisorRunnerEvent{
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
		buf := make([]byte, 32*1024) // 32 KiB buffer like io.Copy
		var total int64
		var e error

		for {
			sr.logger.DebugContext(sr.ctx, "socket->stdout pre-read")
			n, rerr := sr.ioConn.Read(buf)
			sr.logger.DebugContext(sr.ctx, "socket->stdout post-read", "n", n)

			if n > 0 {
				written := 0
				for written < n {
					sr.logger.DebugContext(sr.ctx, "socket->stdout pre-write")
					m, werr := os.Stdout.Write(buf[written:n])
					sr.logger.DebugContext(sr.ctx, "socket->stdout post-write", "m", m)
					if werr != nil {
						sr.logger.ErrorContext(sr.ctx, "socket->stdout write error", "error", werr)
						e = werr
						goto done
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				sr.logger.ErrorContext(sr.ctx, "socket->stdout read error", "error", rerr)
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
			sr.logger.InfoContext(sr.ctx, "socket closed (EOF)")
			trySendEvent(sr.logger, sr.events, SupervisorRunnerEvent{
				ID:   sr.session.Id,
				Type: EvCmdExited,
				Err:  e,
				When: time.Now(),
			})
		} else if e != nil {
			sr.logger.ErrorContext(sr.ctx, "socket->stdout error", "error", e)
			trySendEvent(sr.logger, sr.events, SupervisorRunnerEvent{
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
			sr.logger.WarnContext(sr.ctx, "attachIOSocket: context done")
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
				sr.logger.InfoContext(sr.ctx, "attachIOSocket: normal detach or EOF")
				return nil
			}
			sr.logger.ErrorContext(sr.ctx, "attachIOSocket: error", "error", e)
			return e
		}
	}()

	return nil
}

func (sr *SupervisorRunnerExec) attach() error {
	var response struct{}
	conn, err := sr.sessionClient.Attach(sr.ctx, &sr.id, &response)
	if err != nil {
		sr.logger.ErrorContext(sr.ctx, "attach: failed to attach", "error", err)
		return err
	}
	sr.logger.InfoContext(sr.ctx, "attach: received connection")

	sr.ioConn = conn

	return nil
}

func (sr *SupervisorRunnerExec) forwardResize() error {
	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, err := pty.Getsize(os.Stdin); err == nil {
		ctx, cancel := context.WithTimeout(sr.ctx, 100*time.Millisecond)
		defer cancel()

		if err := sr.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: int(cols), Rows: int(rows)}); err != nil {
			sr.logger.ErrorContext(sr.ctx, "forwardResize: initial resize failed", "error", err)
			return fmt.Errorf("status failed: %w", err)
		}
		sr.logger.InfoContext(sr.ctx, "forwardResize: initial resize sent", "rows", rows, "cols", cols)
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
				sr.logger.WarnContext(sr.ctx, "forwardResize: context done")
				return
			case <-ch:
				// Query current terminal size again on every WINCH
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil {
					sr.logger.WarnContext(sr.ctx, "forwardResize: Getsize failed", "error", err)
					continue
				}
				ctx, cancel := context.WithTimeout(sr.ctx, resizeTimeOut*time.Millisecond)
				defer cancel()
				if err := sr.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: int(cols), Rows: int(rows)}); err != nil {
					sr.logger.ErrorContext(sr.ctx, "forwardResize: resize RPC failed", "error", err)
				} else {
					sr.logger.DebugContext(sr.ctx, "forwardResize: resize sent", "rows", rows, "cols", cols)
				}
			}
		}
	}()
	return nil
}
