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

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrpc"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrunner"
	"github.com/eminwux/sbsh/internal/supervisor/terminalstore"
	"github.com/eminwux/sbsh/pkg/api"
)

/* ---------- Controller ---------- */

// Controller manages the lifecycle of the supervisor.
type Controller struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	logger *slog.Logger

	NewSupervisorRunner func(ctx context.Context, logger *slog.Logger, doc *api.SupervisorDoc, evCh chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner
	NewTerminalStore    func() terminalstore.TerminalStore

	sr supervisorrunner.SupervisorRunner
	ss terminalstore.TerminalStore

	ctrlReadyCh  chan struct{}
	closeReqCh   chan error
	closingCh    chan error
	closedCh     chan struct{}
	shuttingDown atomic.Bool

	eventsCh chan supervisorrunner.Event

	rpcReadyCh chan error
	rpcDoneCh  chan error
}

// NewSupervisorController wires the manager and the shared event channel from terminals.
func NewSupervisorController(ctx context.Context, logger *slog.Logger) api.SupervisorController {
	logger.InfoContext(ctx, "New supervisor controller is being created")
	newCtx, cancel := context.WithCancelCause(ctx)

	c := &Controller{
		ctx:         newCtx,
		cancel:      cancel,
		logger:      logger,
		closedCh:    make(chan struct{}),
		closingCh:   make(chan error, 1),
		ctrlReadyCh: make(chan struct{}),
		rpcReadyCh:  make(chan error),
		rpcDoneCh:   make(chan error),
		closeReqCh:  make(chan error, 1),
		//nolint:mnd // event channel buffer size
		eventsCh:            make(chan supervisorrunner.Event, 32),
		NewSupervisorRunner: supervisorrunner.NewSupervisorRunnerExec,
		NewTerminalStore:    terminalstore.NewTerminalStoreExec,
	}
	return c
}

func (s *Controller) WaitReady() error {
	select {
	case <-s.ctrlReadyCh:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
//
//nolint:gocognit,funlen // long main function
func (s *Controller) Run(doc *api.SupervisorDoc) error {
	supervisorMode := doc.Spec.SupervisorMode
	s.logger.Info("controller loop started", "supervisor_mode", supervisorMode, "run_path", doc.Spec.RunPath)
	defer s.logger.Info("controller loop stopped", "run_path", doc.Spec.RunPath)

	s.sr = s.NewSupervisorRunner(s.ctx, s.logger, doc, s.eventsCh)
	s.ss = s.NewTerminalStore()

	errMetadata := s.sr.CreateMetadata()
	if errMetadata != nil {
		s.logger.Error("failed to write metadata file", "error", errMetadata)
		if errC := s.Close(errMetadata); errC != nil {
			s.logger.Error("error during Close after metadata failure", "error", errC)
			errMetadata = fmt.Errorf("%w: %w :%w", errMetadata, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrWriteMetadata, errMetadata)
	}

	errOpen := s.sr.OpenSocketCtrl()
	if errOpen != nil {
		s.logger.Error("failed to open control socket", "error", errOpen)
		if errC := s.Close(errOpen); errC != nil {
			s.logger.Error("error during Close after socket open failure", "error", errC)
			errOpen = fmt.Errorf("%w: %w :%w", errdefs.ErrOnClose, errOpen, errC)
		}
		close(s.ctrlReadyCh)
		return fmt.Errorf("%w: %w", errdefs.ErrOpenSocketCtrl, errOpen)
	}

	rpc := &supervisorrpc.SupervisorControllerRPC{Core: s}

	go s.sr.StartServer(s.ctx, rpc, s.rpcReadyCh, s.rpcDoneCh)
	// Wait for startup result
	if errRPC := <-s.rpcReadyCh; errRPC != nil {
		if errC := s.Close(errRPC); errC != nil {
			s.logger.Error("error during Close after RPC server start failure", "error", errC)
			errRPC = fmt.Errorf("%w: %w: %w", errRPC, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		s.logger.Error("failed to start supervisor RPC server", "error", errRPC)
		return fmt.Errorf("%w: %w", errdefs.ErrStartRPCServer, errRPC)
	}

	var terminal *api.SupervisedTerminal

	switch supervisorMode {
	case api.RunNewTerminal:
		s.logger.Info("creating new terminal", "spec_id", doc.Spec.ID, "spec_name", doc.Metadata.Name)
		var errCreate error
		terminal, errCreate = s.createRunNewTerminal(doc)
		if errCreate != nil {
			s.logger.Error("failed to create new terminal", "error", errCreate)
			if errC := s.Close(errCreate); errC != nil {
				s.logger.Error("error during Close after new terminal failure", "error", errC)
				errCreate = fmt.Errorf("%w: %w: %w", errCreate, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			// Wrap with appropriate errdefs error if not already wrapped
			if !errors.Is(errCreate, errdefs.ErrNoTerminalSpec) && !errors.Is(errCreate, errdefs.ErrStartCmd) &&
				!errors.Is(errCreate, errdefs.ErrTerminalStore) {
				return fmt.Errorf("%w: %w", errdefs.ErrStartTerminal, errCreate)
			}
			return errCreate
		}

		if errStart := s.sr.StartTerminalCmd(terminal); errStart != nil {
			s.logger.Error("failed to start terminal command", "error", errStart)
			if errC := s.Close(errStart); errC != nil {
				s.logger.Error("error during Close after terminal command failure", "error", errC)
				errStart = fmt.Errorf("%w: %w: %w", errStart, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return fmt.Errorf("%w: %w", errdefs.ErrStartCmd, errStart)
		}
	case api.AttachToTerminal:
		if doc.Spec.TerminalSpec == nil || (doc.Spec.TerminalSpec.ID == "" && doc.Spec.TerminalSpec.Name == "") {
			s.logger.Error("no terminal ID or Name provided for attach")
			errAttachSpec := errdefs.ErrAttachNoTerminalSpec
			if errC := s.Close(errAttachSpec); errC != nil {
				s.logger.Error("error during Close after attach spec failure", "error", errC)
				errAttachSpec = fmt.Errorf("%w: %w: %w", errAttachSpec, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return fmt.Errorf("%w", errAttachSpec)
		}
		s.logger.Info(
			"attaching to existing terminal",
			"attach_id",
			doc.Spec.TerminalSpec.ID,
			"attach_name",
			doc.Spec.TerminalSpec.Name,
		)
		var errAttach error
		terminal, errAttach = s.createAttachTerminal(doc)
		if errAttach != nil {
			s.logger.Error("failed to create attach terminal", "error", errAttach)
			if errC := s.Close(errAttach); errC != nil {
				s.logger.Error("error during Close after attach failure", "error", errC)
				errAttach = fmt.Errorf("%w: %w: %w", errAttach, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return fmt.Errorf("%w: %w", errdefs.ErrAttach, errAttach)
		}

	default:
		s.logger.Error("invalid supervisor mode", "mode", supervisorMode)
		errReturn := errdefs.ErrSupervisorMode
		if errC := s.Close(errdefs.ErrSupervisorMode); errC != nil {
			s.logger.Error("error during Close after invalid kind", "error", errC)
			errReturn = fmt.Errorf("%w: %w: %w", errReturn, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		return errReturn
	}

	if errAttach := s.sr.Attach(terminal); errAttach != nil {
		s.logger.Error("failed to attach to terminal", "error", errAttach)
		if errC := s.Close(errAttach); errC != nil {
			s.logger.Error("error during Close after attach failure", "error", errC)
			errAttach = fmt.Errorf("%w: %w: %w", errAttach, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		return fmt.Errorf("%w: %w", errdefs.ErrAttach, errAttach)
	}

	s.logger.Info("controller ready, entering main event loop")
	close(s.ctrlReadyCh)

	go func() {
		errC := <-s.closingCh
		s.shuttingDown.Store(true)
		s.logger.Warn("controller closing", "reason", errC)
	}()

	for {
		select {
		case <-s.ctx.Done():
			var errDone error
			s.logger.Warn("parent context canceled, shutting down controller")
			if errC := s.Close(s.ctx.Err()); errC != nil {
				s.logger.Error("error during Close after context done", "error", errC)
				errDone = fmt.Errorf("%w: %w", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrContextDone, errDone)

		case ev := <-s.eventsCh:
			s.logger.Debug(
				"received supervisor event",
				"event_id",
				ev.ID,
				"event_type",
				ev.Type,
				"event_err",
				ev.Err,
				"event_time",
				ev.When.Format(time.RFC3339Nano),
			)
			s.handleEvent(ev)

		case errRPC := <-s.rpcDoneCh:
			s.logger.Error("rpc server exited", "error", errRPC)
			if errC := s.Close(errRPC); errC != nil {
				s.logger.Error("error during Close after rpc server failure", "error", errC)
				errRPC = fmt.Errorf("%w: %w: %w", errRPC, errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrRPCServerExited, errRPC)

		case errClose := <-s.closeReqCh:
			s.logger.Warn("close request received", "error", errClose)
			return fmt.Errorf("%w: %w", errdefs.ErrCloseReq, errClose)
		}
	}
}

func (s *Controller) createAttachTerminal(doc *api.SupervisorDoc) (*api.SupervisedTerminal, error) {
	var metadata *api.TerminalDoc
	if doc.Spec.TerminalSpec.ID != "" {
		s.logger.Debug(
			"resolving terminal by id",
			"run_path",
			doc.Spec.RunPath,
			"attach_id",
			string(doc.Spec.TerminalSpec.ID),
		)
		var err error
		metadata, err = discovery.FindTerminalByID(s.ctx, s.logger, doc.Spec.RunPath, string(doc.Spec.TerminalSpec.ID))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errdefs.ErrTerminalNotFoundByID, err)
		}
	} else if doc.Spec.TerminalSpec.Name != "" {
		s.logger.Debug("resolving terminal by name", "run_path", doc.Spec.RunPath, "attach_name", doc.Spec.TerminalSpec.Name)
		var err error
		metadata, err = discovery.FindTerminalByName(s.ctx, s.logger, doc.Spec.RunPath, doc.Spec.TerminalSpec.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errdefs.ErrTerminalNotFoundByName, err)
		}
	}

	if metadata == nil {
		return nil, errdefs.ErrTerminalMetadataNotFound
	}

	terminal := terminalstore.NewSupervisedTerminal(&metadata.Spec)
	if err := s.ss.Add(terminal); err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrTerminalStore, err)
	}
	return terminal, nil
}

func (s *Controller) createRunNewTerminal(doc *api.SupervisorDoc) (*api.SupervisedTerminal, error) {
	if doc.Spec.TerminalSpec == nil {
		return nil, errdefs.ErrNoTerminalSpec
	}

	args := []string{
		"terminal", "-",
	}

	terminal := terminalstore.NewSupervisedTerminal(doc.Spec.TerminalSpec)

	execPath, errExe := os.Executable()
	if errExe != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrStartCmd, errExe)
	}
	terminal.Command = execPath
	terminal.CommandArgs = args

	if errNew := s.ss.Add(terminal); errNew != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrTerminalStore, errNew)
	}

	return terminal, nil
}

/* ---------- Event handlers ---------- */

func (s *Controller) handleEvent(ev supervisorrunner.Event) {
	switch ev.Type {
	case supervisorrunner.EvCmdExited:
		s.logger.InfoContext(s.ctx, "terminal EvCmdExited", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)

	case supervisorrunner.EvError:
		s.logger.ErrorContext(s.ctx, "terminal EvError", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)
	case supervisorrunner.EvDetach:
		s.logger.InfoContext(s.ctx, "terminal EvDetach", "id", ev.ID)
		err := s.Detach()
		if err != nil {
			s.logger.ErrorContext(s.ctx, "failed to detach terminal", "id", ev.ID, "error", err)
		}
	default:
		s.logger.WarnContext(s.ctx, "unknown event type received", "type", ev.Type)
	}
}

func (s *Controller) onClosed(_ api.ID, err error) {
	s.logger.Warn("onClosed called", "err", err)
	_ = s.Close(err)
}

func (s *Controller) Close(reason error) error {
	if !s.shuttingDown.Load() {
		s.logger.Info("initiating shutdown sequence", "reason", reason)
		// Set closing reason
		s.closingCh <- reason

		// Notify terminal runner to close all terminals
		_ = s.sr.Close(reason)

		// Notify Run to exit
		s.closeReqCh <- reason

		// Mark controller as closed
		close(s.closedCh)
	} else {
		s.logger.Info("shutdown sequence already in progress, ignoring duplicate request", "reason", reason)
	}
	return nil
}

func (s *Controller) WaitClose() error {
	select {
	case <-s.closedCh:
		s.logger.Debug("controller has fully exited and resources are released")
		return nil
	}
}

func (s *Controller) Detach() error {
	// Request detach from terminal
	if err := s.sr.Detach(); err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrDetachTerminal, err)
	}
	return nil
}
