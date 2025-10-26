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
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/internal/dualcopier"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/rpcclient/session"
)

const (
	// in milliseconds.
	resizeTimeout = 100
)

func (sr *Exec) dialSessionCtrlSocket() error {
	sr.logger.DebugContext(sr.ctx, "dialSessionCtrlSocket: connecting to session",
		"session_id", sr.session.Spec.ID,
		"socket_file", sr.session.Spec.SocketFile)

	sr.sessionClient = session.NewUnix(sr.session.Spec.SocketFile, sr.logger)
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

func (sr *Exec) startConnectionManager() error {
	// Connected, now we enable raw mode
	if err := sr.toBashUIMode(); err != nil {
		sr.logger.ErrorContext(sr.ctx, "attachIOSocket: initial raw mode failed", "error", err)
	}

	// We want half-closes; UnixConn exposes CloseRead/CloseWrite
	uc, ok := sr.ioConn.(*net.UnixConn)
	if !ok {
		sr.logger.ErrorContext(sr.ctx, "StartConnManager: ioConn is not a *net.UnixConn")
		return errors.New("ioConn is not a *net.UnixConn")
	}

	dc := dualcopier.NewCopier(sr.ctx, sr.logger)

	// WRITER: stdin -> socket
	readyWriter := make(chan struct{})
	go dc.RunCopier(os.Stdin, sr.ioConn, readyWriter, func() {
		if uc != nil {
			sr.logger.DebugContext(sr.ctx, "stdin->socket: closing write side of UnixConn")
			_ = uc.CloseWrite()
		}
	})

	// READER: socket  -> stdout
	readyReader := make(chan struct{})
	go dc.RunCopier(sr.ioConn, os.Stdout, readyReader, func() {
		if uc != nil {
			sr.logger.DebugContext(sr.ctx, "socket->stdout: closing read side of UnixConn")
			_ = uc.CloseRead()
		}
	})

	<-readyWriter
	<-readyReader

	// MANAGER
	go dc.CopierManager(uc, func() {
		trySendEvent(sr.logger, sr.events, Event{
			ID:   sr.session.Spec.ID,
			Type: EvError,
			Err:  errors.New("read/write routines exited"),
			When: time.Now(),
		})
	})

	// // Force an initial terminal resize event to ensure the session starts with correct dimensions
	// sr.logger.DebugContext(sr.ctx, "attachIOSocket: sending initial SIGWINCH to self")
	// if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
	// 	sr.logger.WarnContext(sr.ctx, "attachIOSocket: failed to send SIGWINCH", "error", err)
	// }

	return nil
}

func (sr *Exec) attach() error {
	var response struct{}
	conn, err := sr.sessionClient.Attach(sr.ctx, &sr.id, &response)
	if err != nil {
		sr.logger.ErrorContext(sr.ctx, "attach: failed to attach", "error", err)
		return err
	}
	sr.logger.InfoContext(sr.ctx, "attach: received connection")

	sr.metadata.Status.State = api.SupervisorAttached
	errM := sr.updateMetadata()
	if errM != nil {
		sr.logger.Error("failed to update metadata", "error", errM)
		return errM
	}
	sr.logger.Info("metadata created successfully")
	sr.ioConn = conn

	return nil
}

func (sr *Exec) forwardResize() error {
	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, errSize := pty.Getsize(os.Stdin); errSize == nil {
		const resizeTimeout = 100 * time.Millisecond
		ctx, cancel := context.WithTimeout(sr.ctx, resizeTimeout)
		defer cancel()

		if err := sr.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: cols, Rows: rows}); err != nil {
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
				ctx, cancel := context.WithTimeout(sr.ctx, resizeTimeout*time.Millisecond)
				defer cancel()
				if errResize := sr.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: cols, Rows: rows}); errResize != nil {
					sr.logger.ErrorContext(sr.ctx, "forwardResize: resize RPC failed", "error", errResize)
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

func (sr *Exec) waitReady() error {
	ctx, cancel := context.WithTimeout(sr.ctx, waitReadyTimeoutSeconds*time.Second)
	defer cancel()

	sr.logger.InfoContext(
		sr.ctx,
		"waitReady: waiting for session to be ready",
		"session_id",
		sr.session.Spec.ID,
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
				sr.session.Spec.ID,
				"session_name",
				sr.metadata.Spec.Name,
			)
		}
	}
}
