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
	"golang.org/x/sync/errgroup"
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

func (sr *SupervisorRunnerExec) readWriteBytes(r io.Reader, w io.Writer) error {
	//nolint:mnd // 32 KiB buffer
	buf := make([]byte, 32*1024)
	var total int64
	n, rerr := r.Read(buf)
	sr.logger.DebugContext(sr.ctx, "stdin->socket post-read", "n", n)

	if n > 0 {
		written := 0
		for written < n {
			sr.logger.DebugContext(sr.ctx, "stdin->socket pre-write")
			m, werr := w.Write(buf[written:n])
			sr.logger.DebugContext(sr.ctx, "stdin->socket post-write", "m", m)
			if werr != nil {
				return fmt.Errorf("could not write stdin->socket: %w", werr)
			}
			written += m
			total += int64(m)
		}
	}

	if rerr != nil {
		sr.logger.ErrorContext(sr.ctx, "stdin->socket read error", "error", rerr)
		return fmt.Errorf("could not read stdin->socket: %w", rerr)
	}

	return nil
}

func (sr *SupervisorRunnerExec) connManager(ctx context.Context, g *errgroup.Group, uc *net.UnixConn) {
	errGroup := make(chan error, 1)
	go func() {
		errGroup <- g.Wait()
	}()

	select {
	case <-ctx.Done():
		sr.logger.InfoContext(sr.ctx, "connManager: context cancelled or error received, beginning shutdown")
		// ACT IMMEDIATELY: close/unblock I/O, log, metrics, etc.
		//nolint:nestif // thorough debug
		if uc != nil {
			if err := uc.SetReadDeadline(time.Now()); err != nil {
				sr.logger.WarnContext(sr.ctx, "connManager: failed to set read deadline", "error", err)
			} else {
				sr.logger.DebugContext(sr.ctx, "connManager: set read deadline to unblock readers")
			}
			if err := uc.SetWriteDeadline(time.Now()); err != nil {
				sr.logger.WarnContext(sr.ctx, "connManager: failed to set write deadline", "error", err)
			} else {
				sr.logger.DebugContext(sr.ctx, "connManager: set write deadline to unblock writers")
			}
		} else {
			sr.logger.WarnContext(sr.ctx, "connManager: UnixConn is nil, cannot set deadlines")
		}
		sr.logger.InfoContext(sr.ctx, "connManager: context done, shutting down connection manager")
		trySendEvent(sr.logger, sr.events, SupervisorRunnerEvent{
			ID:   sr.session.Id,
			Type: EvError,
			Err:  errors.New("read/write routines exited"),
			When: time.Now(),
		})

	case err := <-errGroup:
		e := fmt.Errorf("wait on routines error group returned: %w", err)
		sr.logger.ErrorContext(sr.ctx, "read/write routines finished", "error", e)
	}
}

func (sr *SupervisorRunnerExec) runReadWriter(ctx context.Context, uc *net.UnixConn, r io.Reader, w io.Writer) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sr.logger.DebugContext(
				sr.ctx,
				"pre-read",
				"reader",
				fmt.Sprintf("%T", r),
				"writer",
				fmt.Sprintf("%T", w),
			)
			err := sr.readWriteBytes(r, w)
			if err != nil {
				if uc != nil {
					_ = uc.CloseRead()
				}
				return err
			}
		}
	}
}

func (sr *SupervisorRunnerExec) StartConnManager() error {
	// Connected, now we enable raw mode
	if err := sr.toBashUIMode(); err != nil {
		sr.logger.ErrorContext(sr.ctx, "attachIOSocket: initial raw mode failed", "error", err)
	}

	// We want half-closes; UnixConn exposes CloseRead/CloseWrite
	uc, _ := sr.ioConn.(*net.UnixConn)

	g, ctx := errgroup.WithContext(sr.ctx)

	// WRITER: stdin -> socket
	g.Go(func() error {
		return sr.runReadWriter(ctx, uc, os.Stdin, sr.ioConn)
	})

	// READER: socket  -> stdin
	g.Go(func() error {
		return sr.runReadWriter(ctx, uc, sr.ioConn, os.Stdout)
	})

	// Spawn ConnManager
	go sr.connManager(ctx, g, uc)

	// Force an initial terminal resize event to ensure the session starts with correct dimensions
	sr.logger.DebugContext(sr.ctx, "attachIOSocket: sending initial SIGWINCH to self")
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		sr.logger.WarnContext(sr.ctx, "attachIOSocket: failed to send SIGWINCH", "error", err)
	}

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

const (
	waitReadyTimeoutSeconds   = 2
	waitReadyTickMilliseconds = 50
)

func (sr *SupervisorRunnerExec) waitReady() error {
	ctx, cancel := context.WithTimeout(sr.ctx, waitReadyTimeoutSeconds*time.Second)
	defer cancel()

	sr.logger.InfoContext(
		sr.ctx,
		"waitReady: waiting for session to be ready",
		"session_id",
		sr.session.Id,
		"session_name",
		sr.metadata.Spec.Name,
	)

	ticker := time.NewTicker(waitReadyTickMilliseconds * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sr.logger.ErrorContext(sr.ctx, "waitReady: context done before ready", "error", ctx.Err())
			return fmt.Errorf("context done before ready: %w", ctx.Err())
		case <-ticker.C:
			// refresh metadata
			metadata, err := sr.getSessionMetadata()
			if err != nil {
				sr.logger.ErrorContext(sr.ctx, "waitReady: getSessionMetadata failed during wait", "error", err)
				return fmt.Errorf("get session metadata failed during wait: %w", err)
			}
			if metadata.Status.State == api.Ready {
				sr.logger.InfoContext(
					sr.ctx,
					"waitReady: session is ready",
					"session_id",
					metadata.Spec.ID,
					"session_name",
					metadata.Spec.Name,
				)
				return nil
			}
			sr.logger.DebugContext(
				sr.ctx,
				"waitReady: session not ready yet",
				"session_id",
				sr.session.Id,
				"session_name",
				sr.metadata.Spec.Name,
			)
		}
	}
}

func (sr *SupervisorRunnerExec) getSessionMetadata() (*api.SessionMetadata, error) {
	if sr.sessionClient == nil {
		return nil, errors.New("getSessionMetadata: session client is nil")
	}

	var metadata api.SessionMetadata
	if err := sr.sessionClient.Metadata(sr.ctx, &metadata); err != nil {
		sr.logger.ErrorContext(sr.ctx, "getSessionMetadata: failed to get metadata", "error", err)
		return nil, fmt.Errorf("get metadata RPC failed: %w", err)
	}
	sr.logger.InfoContext(sr.ctx, "getSessionMetadata: metadata retrieved", "metadata", metadata)
	return &metadata, nil
}
