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

	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/session/sessionrpc"
	"sbsh/pkg/session/sessionrunner"
)

type SessionController struct {
	ctx context.Context

	closedCh  chan struct{}
	closingCh chan error

	shutttingDown bool
}

var (
	newSessionRunner = sessionrunner.NewSessionRunnerExec
	sr               sessionrunner.SessionRunner
	rpcReadyCh       chan error                            = make(chan error)
	rpcDoneCh        chan error                            = make(chan error)
	ctrlReady        chan struct{}                         = make(chan struct{}, 1)
	eventsCh         chan sessionrunner.SessionRunnerEvent = make(chan sessionrunner.SessionRunnerEvent, 32)
	closeReqCh       chan error                            = make(chan error, 1)
)

// NewSessionController wires the manager and the shared event channel from sessions.
func NewSessionController(ctx context.Context) api.SessionController {
	slog.Debug("[sessionCtrl] New controller is being created\r\n")

	c := &SessionController{
		ctx:       ctx,
		closedCh:  make(chan struct{}),
		closingCh: make(chan error, 1),
	}

	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady() error {
	select {
	case <-ctrlReady:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *SessionController) WaitClose() error {
	select {
	case <-c.closedCh:
		slog.Debug("controller exited")
		return nil
	case err := <-c.closingCh:
		slog.Debug("controller closing", "reason", err)
	}

	return nil
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run(spec *api.SessionSpec) error {
	defer slog.Debug("[sessionCtrl] controller stopped\r\n")

	sr = newSessionRunner(c.ctx, spec)

	if len(spec.Command) == 0 {
		slog.Debug("empty command in SessionSpec")
		return errdefs.ErrSpecCmdMissing
	}

	slog.Debug("[sessionCtrl] Starting controller loop")

	err := sr.CreateMetadata()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not write metadata file: %v", err))
		if err := c.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w: %v", errdefs.ErrWriteMetadata, err)
	}

	err = sr.OpenSocketCtrl()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not open control socket: %v", err))
		if err := c.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w: %v", errdefs.ErrOpenSocketCtrl, err)
	}

	rpc := &sessionrpc.SessionControllerRPC{Core: c}
	go sr.StartServer(c.ctx, rpc, rpcReadyCh, rpcDoneCh)
	// Wait for startup result
	if err := <-rpcReadyCh; err != nil {
		// failed to start — handle and return
		slog.Debug(fmt.Sprintf("failed to start server: %v", err))
		return fmt.Errorf("%w: %v", errdefs.ErrStartRPCServer, err)
	}

	if err := sr.StartSession(eventsCh); err != nil {
		slog.Debug(fmt.Sprintf("failed to start session: %v", err))
		if err := c.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w: %v", errdefs.ErrStartSession, err)
	}

	// ctrlReady <- struct{}{}
	close(ctrlReady)

	go func() {
		select {
		case err := <-c.closingCh:
			c.shutttingDown = true
			slog.Info("controller closing", "reason", err)
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			var err error
			slog.Debug("[supervisor] parent context channel has been closed\r\n")
			if errC := c.Close(c.ctx.Err()); errC != nil {
				err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %v", errdefs.ErrContextDone, err)

		case ev := <-eventsCh:
			slog.Debug(
				fmt.Sprintf(
					"[sessionCtrl] received event: id=%s type=%v err=%v when=%s\r\n",
					ev.ID,
					ev.Type,
					ev.Err,
					ev.When.Format(time.RFC3339Nano),
				),
			)
			c.handleEvent(ev)

		case err := <-rpcDoneCh:
			slog.Debug(fmt.Sprintf("[sessionCtrl] rpc server has failed: %v\r\n", err))
			if errC := c.Close(err); err != nil {
				err = fmt.Errorf("%w: %v: %v", err, errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %v", errdefs.ErrRPCServerExited, err)

		case err := <-closeReqCh:
			slog.Debug(fmt.Sprintf("[sessionCtrl] close request received: %v\r\n", err))
			return fmt.Errorf("%w: %v", errdefs.ErrCloseReq, err)
		}
	}
}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev sessionrunner.SessionRunnerEvent) {
	switch ev.Type {

	case sessionrunner.EvError:
		slog.Debug(fmt.Sprintf("[sessionCtrl] session %s EvError error: %v\r\n", ev.ID, ev.Err))
		c.onClosed(ev.Err)

	case sessionrunner.EvCmdExited:
		slog.Debug(fmt.Sprintf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err))
		c.onClosed(ev.Err)
	}
}

func (c *SessionController) Close(reason error) error {
	if !c.shutttingDown {
		slog.Info("initiating shutdown sequence", "reason", reason)
		// Set closing reason
		c.closingCh <- reason

		// Notify session runner to close all sessions
		sr.Close(reason)

		// Notify Run to exit
		closeReqCh <- reason

		// Mark controller as closed
		close(c.closedCh)
	} else {
		slog.Info("shutdown sequence already in progress, ignoring duplicate request", "reason", reason)
	}
	return nil
}

func (c *SessionController) onClosed(err error) {
	slog.Debug("[sessionCtrl] onClosed triggered\r\n")
	c.Close(err)
}

func (c *SessionController) Resize(args api.ResizeArgs) {
	sr.Resize(args)
}

func (c *SessionController) Attach(id *api.ID, response *api.ResponseWithFD) error {
	var err error
	err = sr.Attach(id, response)
	if err != nil {
		return err
	}

	slog.Debug("[session] Attach controller response",
		"ok", response.JSON,
		"fds", response.FDs,
	)

	return nil
}

func (c *SessionController) Detach(id *api.ID) error {
	return sr.Detach(id)
}
