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
	logger.DebugContext(ctx, "[sessionCtrl] New controller is being created\r\n")

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
	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady() error {
	select {
	case <-c.ctrlReadyCh:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *SessionController) WaitClose() error {
	select {
	case <-c.closedCh:
		c.logger.DebugContext(c.ctx, "controller exited")
		return nil
	case err := <-c.closingCh:
		c.logger.DebugContext(c.ctx, "controller closing", "reason", err)
		return nil
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run(spec *api.SessionSpec) error {
	defer c.logger.DebugContext(c.ctx, "[sessionCtrl] controller stopped\r\n")

	c.sr = c.NewSessionRunner(c.ctx, c.logger, spec)

	if len(spec.Command) == 0 {
		c.logger.DebugContext(c.ctx, "empty command in SessionSpec")
		return errdefs.ErrSpecCmdMissing
	}

	c.logger.DebugContext(c.ctx, "[sessionCtrl] Starting controller loop")

	err := c.sr.CreateMetadata()
	if err != nil {
		c.logger.DebugContext(c.ctx, fmt.Sprintf("could not write metadata file: %v", err))
		if errB := c.Close(err); errB != nil {
			err = fmt.Errorf("%w: %w :%w", err, errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrWriteMetadata, err)
	}

	err = c.sr.OpenSocketCtrl()
	if err != nil {
		c.logger.DebugContext(c.ctx, fmt.Sprintf("could not open control socket: %v", err))
		if errC := c.Close(err); errC != nil {
			err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrOpenSocketCtrl, err)
	}

	rpc := &sessionrpc.SessionControllerRPC{Core: c}
	go c.sr.StartServer(c.ctx, rpc, c.rpcReadyCh, c.rpcDoneCh)
	// Wait for startup result
	if errC := <-c.rpcReadyCh; errC != nil {
		// failed to start — handle and return
		c.logger.DebugContext(c.ctx, fmt.Sprintf("failed to start server: %v", errC))
		return fmt.Errorf("%w: %w", errdefs.ErrStartRPCServer, errC)
	}

	if errB := c.sr.StartSession(c.eventsCh); errB != nil {
		c.logger.DebugContext(c.ctx, fmt.Sprintf("failed to start session: %v", errB))
		if errC := c.Close(errB); errC != nil {
			errB = fmt.Errorf("%w: %w: %w", errB, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrStartSession, errB)
	}

	close(c.ctrlReadyCh)

	go func() {
		select {
		case err := <-c.closingCh:
			c.shuttingDown = true
			slog.Info("controller closing", "reason", err)
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			var err error
			c.logger.DebugContext(c.ctx, "[supervisor] parent context channel has been closed\r\n")
			if errC := c.Close(c.ctx.Err()); errC != nil {
				err = fmt.Errorf("%w: %w", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)

		case ev := <-c.eventsCh:
			c.logger.DebugContext(c.ctx,
				fmt.Sprintf(
					"[sessionCtrl] received event: id=%s type=%v err=%v when=%s\r\n",
					ev.ID,
					ev.Type,
					ev.Err,
					ev.When.Format(time.RFC3339Nano),
				),
			)
			c.handleEvent(ev)

		case err := <-c.rpcDoneCh:
			c.logger.DebugContext(c.ctx, fmt.Sprintf("[sessionCtrl] rpc server has failed: %v\r\n", err))
			if errC := c.Close(err); err != nil {
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrRPCServerExited, err)

		case err := <-c.closeReqCh:
			c.logger.DebugContext(c.ctx, fmt.Sprintf("[sessionCtrl] close request received: %v\r\n", err))
			return fmt.Errorf("%w: %w", errdefs.ErrCloseReq, err)
		}
	}
}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev sessionrunner.SessionRunnerEvent) {
	switch ev.Type {
	case sessionrunner.EvError:
		c.logger.DebugContext(c.ctx, fmt.Sprintf("[sessionCtrl] session %s EvError error: %v\r\n", ev.ID, ev.Err))
		c.onClosed(ev.Err)

	case sessionrunner.EvCmdExited:
		c.logger.DebugContext(
			c.ctx,
			fmt.Sprintf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err),
		)
		c.onClosed(ev.Err)
	}
}

func (c *SessionController) Close(reason error) error {
	if !c.shuttingDown {
		slog.Info("initiating shutdown sequence", "reason", reason)
		// Set closing reason
		c.closingCh <- reason

		// Notify session runner to close all sessions
		c.sr.Close(reason)

		// Notify Run to exit
		c.closeReqCh <- reason

		// Mark controller as closed
		close(c.closedCh)
	} else {
		slog.Info("shutdown sequence already in progress, ignoring duplicate request", "reason", reason)
	}
	return nil
}

func (c *SessionController) onClosed(err error) {
	c.logger.DebugContext(c.ctx, "[sessionCtrl] onClosed triggered\r\n")
	c.Close(err)
}

func (c *SessionController) Resize(args api.ResizeArgs) {
	c.sr.Resize(args)
}

func (c *SessionController) Attach(id *api.ID, response *api.ResponseWithFD) error {
	err := c.sr.Attach(id, response)
	if err != nil {
		return err
	}

	c.logger.DebugContext(c.ctx, "[session] Attach controller response",
		"ok", response.JSON,
		"fds", response.FDs,
	)

	return nil
}

func (c *SessionController) Detach(id *api.ID) error {
	return c.sr.Detach(id)
}
