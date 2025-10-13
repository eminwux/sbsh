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

package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/session/sessionrpc"
	"github.com/eminwux/sbsh/internal/session/sessionrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

type SessionController struct {
	ctx    context.Context
	logger *slog.Logger

	NewSessionRunner func(ctx context.Context, logger *slog.Logger, spec *api.SessionSpec) sessionrunner.SessionRunner

	sr sessionrunner.SessionRunner

	ctrlReadyCh  chan struct{}
	closeReqCh   chan error
	closingCh    chan error
	closedCh     chan struct{}
	shuttingDown bool

	eventsCh chan sessionrunner.SessionRunnerEvent

	rpcReadyCh chan error
	rpcDoneCh  chan error
}

// var newSessionRunner = sessionrunner.NewSessionRunnerExec

// NewSessionController wires the manager and the shared event channel from sessions.
func NewSessionController(ctx context.Context, logger *slog.Logger) api.SessionController {
	logger.DebugContext(ctx, "New controller is being created")

	c := &SessionController{
		ctx:         ctx,
		logger:      logger,
		closedCh:    make(chan struct{}),
		closingCh:   make(chan error, 1),
		rpcReadyCh:  make(chan error),
		rpcDoneCh:   make(chan error),
		ctrlReadyCh: make(chan struct{}, 1),

		//nolint:mnd // event channel buffer size
		eventsCh:         make(chan sessionrunner.SessionRunnerEvent, 32),
		closeReqCh:       make(chan error, 1),
		NewSessionRunner: sessionrunner.NewSessionRunnerExec,
	}
	logger.InfoContext(ctx, "SessionController created")
	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady() error {
	select {
	case <-c.ctrlReadyCh:
		c.logger.DebugContext(c.ctx, "controller is ready")
		return nil
	case <-c.ctx.Done():
		c.logger.WarnContext(c.ctx, "WaitReady: context done", "error", c.ctx.Err())
		return c.ctx.Err()
	}
}

func (c *SessionController) WaitClose() error {
	select {
	case <-c.closedCh:
		c.logger.InfoContext(c.ctx, "controller exited")
		return nil
	case err := <-c.closingCh:
		c.logger.WarnContext(c.ctx, "controller closing", "reason", err)
	}
	return nil
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run(spec *api.SessionSpec) error {
	defer c.logger.InfoContext(c.ctx, "controller stopped")

	c.logger.DebugContext(c.ctx, "creating new session runner")
	c.sr = c.NewSessionRunner(c.ctx, c.logger, spec)

	if len(spec.Command) == 0 {
		c.logger.ErrorContext(c.ctx, "empty command in SessionSpec")
		return errdefs.ErrSpecCmdMissing
	}

	c.logger.InfoContext(c.ctx, "Starting controller loop")

	err := c.sr.CreateMetadata()
	if err != nil {
		c.logger.ErrorContext(c.ctx, "could not write metadata file", "err", err)
		if errB := c.Close(err); errB != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after metadata failure", "err", errB)
			err = fmt.Errorf("%w: %w :%w", err, errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrWriteMetadata, err)
	}

	err = c.sr.OpenSocketCtrl()
	if err != nil {
		c.logger.ErrorContext(c.ctx, "could not open control socket", "err", err)
		if errC := c.Close(err); errC != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after socket open failure", "err", errC)
			err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrOpenSocketCtrl, err)
	}

	rpc := &sessionrpc.SessionControllerRPC{Core: c}
	go c.sr.StartServer(c.ctx, rpc, c.rpcReadyCh, c.rpcDoneCh)
	// Wait for startup result
	if errC := <-c.rpcReadyCh; errC != nil {
		c.logger.ErrorContext(c.ctx, "failed to start server", "err", errC)
		return fmt.Errorf("%w: %w", errdefs.ErrStartRPCServer, errC)
	}

	if errB := c.sr.StartSession(c.eventsCh); errB != nil {
		c.logger.ErrorContext(c.ctx, "failed to start session", "err", errB)
		if errC := c.Close(errB); errC != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after session start failure", "err", errC)
			errB = fmt.Errorf("%w: %w: %w", errB, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrStartSession, errB)
	}

	c.logger.InfoContext(c.ctx, "controller ready")
	close(c.ctrlReadyCh)

	go func() {
		select {
		case err := <-c.closingCh:
			c.shuttingDown = true
			c.logger.WarnContext(c.ctx, "controller closing", "reason", err)
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			var err error
			c.logger.WarnContext(c.ctx, "parent context channel has been closed")
			if errC := c.Close(c.ctx.Err()); errC != nil {
				c.logger.ErrorContext(c.ctx, "error during Close after context done", "err", errC)
				err = fmt.Errorf("%w: %w", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)

		case ev := <-c.eventsCh:
			c.logger.DebugContext(c.ctx,
				fmt.Sprintf(
					"received event: id=%s type=%v err=%v when=%s",
					ev.ID,
					ev.Type,
					ev.Err,
					ev.When.Format(time.RFC3339Nano),
				),
			)
			c.handleEvent(ev)

		case err := <-c.rpcDoneCh:
			c.logger.ErrorContext(c.ctx, "rpc server has failed", "err", err)
			if errC := c.Close(err); err != nil {
				c.logger.ErrorContext(c.ctx, "error during Close after rpc server failure", "err", errC)
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrRPCServerExited, err)

		case err := <-c.closeReqCh:
			c.logger.WarnContext(c.ctx, "close request received", "err", err)
			return fmt.Errorf("%w: %w", errdefs.ErrCloseReq, err)
		}
	}
}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev sessionrunner.SessionRunnerEvent) {
	switch ev.Type {
	case sessionrunner.EvError:
		c.logger.ErrorContext(c.ctx, "session EvError", "id", ev.ID, "error", ev.Err)
		c.onClosed(ev.Err)

	case sessionrunner.EvCmdExited:
		c.logger.InfoContext(c.ctx, "session EvSessionExited", "id", ev.ID, "error", ev.Err)
		c.onClosed(ev.Err)
	}
}

func (c *SessionController) Close(reason error) error {
	if !c.shuttingDown {
		c.logger.InfoContext(c.ctx, "initiating shutdown sequence", "reason", reason)
		// Set closing reason
		c.closingCh <- reason

		// Notify session runner to close all sessions
		_ = c.sr.Close(reason)

		// Notify Run to exit
		c.closeReqCh <- reason

		// Mark controller as closed
		close(c.closedCh)
	} else {
		c.logger.WarnContext(c.ctx, "shutdown sequence already in progress, ignoring duplicate request", "reason", reason)
	}
	return nil
}

func (c *SessionController) onClosed(err error) {
	c.logger.InfoContext(c.ctx, "onClosed triggered")
	_ = c.Close(err)
}

func (c *SessionController) Resize(args api.ResizeArgs) {
	c.sr.Resize(args)
}

func (c *SessionController) Attach(id *api.ID, response *api.ResponseWithFD) error {
	err := c.sr.Attach(id, response)
	if err != nil {
		c.logger.ErrorContext(c.ctx, "Attach failed", "id", id, "error", err)
		return err
	}

	c.logger.InfoContext(c.ctx, "Attach controller response", "ok", response.JSON, "fds", response.FDs)
	return nil
}

func (c *SessionController) Detach(id *api.ID) error {
	return c.sr.Detach(id)
}
