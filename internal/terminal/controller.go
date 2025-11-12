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

package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

type Controller struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	logger *slog.Logger

	NewTerminalRunner func(ctx context.Context, logger *slog.Logger, spec *api.TerminalSpec) terminalrunner.TerminalRunner

	sr terminalrunner.TerminalRunner

	ctrlReadyCh  chan struct{}
	closeReqCh   chan error
	closingCh    chan error
	closedCh     chan struct{}
	shuttingDown bool

	eventsCh chan terminalrunner.Event

	rpcReadyCh chan error
	rpcDoneCh  chan error
}

// NewTerminalController wires the manager and the shared event channel from terminals.
func NewTerminalController(ctx context.Context, logger *slog.Logger) api.TerminalController {
	logger.DebugContext(ctx, "New terminal controller is being created")
	newCtx, cancel := context.WithCancelCause(ctx)

	c := &Controller{
		ctx:         newCtx,
		cancel:      cancel,
		logger:      logger,
		closedCh:    make(chan struct{}),
		closingCh:   make(chan error, 1),
		rpcReadyCh:  make(chan error),
		rpcDoneCh:   make(chan error),
		ctrlReadyCh: make(chan struct{}, 1),

		//nolint:mnd // event channel buffer size
		eventsCh:          make(chan terminalrunner.Event, 32),
		closeReqCh:        make(chan error, 1),
		NewTerminalRunner: terminalrunner.NewTerminalRunnerExec,
	}
	logger.InfoContext(ctx, "TerminalController created")
	return c
}

func (c *Controller) Ping(in *api.PingMessage) (*api.PingMessage, error) {
	if in.Message == "PING" {
		return &api.PingMessage{Message: "PONG"}, nil
	}
	return &api.PingMessage{}, fmt.Errorf("unexpected ping message: %s", in.Message)
}

func (c *Controller) WaitReady() error {
	select {
	case <-c.ctrlReadyCh:
		c.logger.DebugContext(c.ctx, "controller is ready")
		return nil
	case <-c.ctx.Done():
		c.logger.WarnContext(c.ctx, "WaitReady: context done", "error", c.ctx.Err())
		return c.ctx.Err()
	}
}

func (c *Controller) WaitClose() error {
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
//
//nolint:gocognit,funlen // long main function
func (c *Controller) Run(spec *api.TerminalSpec) error {
	defer c.logger.InfoContext(c.ctx, "controller stopped")

	c.logger.DebugContext(c.ctx, "creating new terminal runner")
	c.sr = c.NewTerminalRunner(c.ctx, c.logger, spec)

	if len(spec.Command) == 0 {
		c.logger.ErrorContext(c.ctx, "empty command in TerminalSpec")
		return errdefs.ErrSpecCmdMissing
	}

	c.logger.InfoContext(c.ctx, "Starting controller loop")

	errMetadata := c.sr.CreateMetadata()
	if errMetadata != nil {
		c.logger.ErrorContext(c.ctx, "could not write metadata file", "err", errMetadata)
		if errB := c.Close(errMetadata); errB != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after metadata failure", "err", errB)
			errMetadata = fmt.Errorf("%w: %w :%w", errMetadata, errdefs.ErrOnClose, errB)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrWriteMetadata, errMetadata)
	}

	errOpen := c.sr.OpenSocketCtrl()
	if errOpen != nil {
		c.logger.ErrorContext(c.ctx, "could not open control socket", "err", errOpen)
		if errC := c.Close(errOpen); errC != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after socket open failure", "err", errC)
			errOpen = fmt.Errorf("%w: %w: %w", errOpen, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrOpenSocketCtrl, errOpen)
	}

	rpc := &terminalrpc.TerminalControllerRPC{Core: c}
	go c.sr.StartServer(c.ctx, rpc, c.rpcReadyCh, c.rpcDoneCh)
	// Wait for startup result
	if errC := <-c.rpcReadyCh; errC != nil {
		c.logger.ErrorContext(c.ctx, "failed to start server", "err", errC)
		return fmt.Errorf("%w: %w", errdefs.ErrStartRPCServer, errC)
	}

	if errStart := c.sr.StartTerminal(c.eventsCh); errStart != nil {
		c.logger.ErrorContext(c.ctx, "failed to start terminal", "err", errStart)
		if errC := c.Close(errStart); errC != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after terminal start failure", "err", errC)
			errStart = fmt.Errorf("%w: %w: %w", errStart, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrStartTerminal, errStart)
	}

	if errSetup := c.sr.SetupShell(); errSetup != nil {
		c.logger.ErrorContext(c.ctx, "failed to setup shell", "err", errSetup)
		if errClose := c.Close(errSetup); errClose != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after setup shell failure", "err", errClose)
			errSetup = fmt.Errorf("%w: %w: %w", errSetup, errdefs.ErrOnClose, errClose)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrSetupShell, errSetup)
	}

	if errInit := c.sr.OnInitShell(); errInit != nil {
		c.logger.ErrorContext(c.ctx, "failed on init shell", "err", errInit)
		if errClose := c.Close(errInit); errClose != nil {
			c.logger.ErrorContext(c.ctx, "error during Close after init shell failure", "err", errClose)
			errInit = fmt.Errorf("%w: %w: %w", errInit, errdefs.ErrOnClose, errClose)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrInitShell, errInit)
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

func (c *Controller) handleEvent(ev terminalrunner.Event) {
	switch ev.Type {
	case terminalrunner.EvError:
		c.logger.ErrorContext(c.ctx, "terminal EvError", "id", ev.ID, "error", ev.Err)
		c.onClosed(ev.Err)

	case terminalrunner.EvCmdExited:
		c.logger.InfoContext(c.ctx, "terminal EvCmdExited", "id", ev.ID, "error", ev.Err)
		c.onClosed(ev.Err)
	default:
		c.logger.WarnContext(c.ctx, "unknown event type received", "type", ev.Type)
	}
}

func (c *Controller) Close(reason error) error {
	if !c.shuttingDown {
		c.logger.InfoContext(c.ctx, "initiating shutdown sequence", "reason", reason)

		// cancel the controller context so WaitReady unblocks
		c.cancel(reason)

		// Set closing reason
		c.closingCh <- reason

		// Notify terminal runner to close all terminals
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

func (c *Controller) onClosed(err error) {
	c.logger.InfoContext(c.ctx, "onClosed triggered")
	_ = c.Close(err)
}

func (c *Controller) Resize(args api.ResizeArgs) {
	c.sr.Resize(args)
}

func (c *Controller) Attach(id *api.ID, response *api.ResponseWithFD) error {
	err := c.sr.Attach(id, response)
	if err != nil {
		c.logger.ErrorContext(c.ctx, "Attach failed", "id", id, "error", err)
		return err
	}

	errPostAttach := c.sr.PostAttachShell()
	if errPostAttach != nil {
		c.logger.ErrorContext(c.ctx, "Attach failed", "id", id, "error", errPostAttach)
		return errPostAttach
	}

	c.logger.InfoContext(c.ctx, "Attach controller response", "ok", response.JSON, "fds", response.FDs)
	return nil
}

func (c *Controller) Detach(id *api.ID) error {
	return c.sr.Detach(id)
}

func (c *Controller) Metadata() (*api.TerminalMetadata, error) {
	return c.sr.Metadata()
}

func (c *Controller) State() (*api.TerminalStatusMode, error) {
	metadata, err := c.sr.Metadata()
	if err != nil {
		return nil, err
	}
	return &metadata.Status.State, nil
}
