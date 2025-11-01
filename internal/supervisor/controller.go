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
	"time"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/supervisor/sessionstore"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrpc"
	"github.com/eminwux/sbsh/internal/supervisor/supervisorrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

/* ---------- Controller ---------- */

// Controller manages the lifecycle of the supervisor.
type Controller struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	logger *slog.Logger

	NewSupervisorRunner func(ctx context.Context, logger *slog.Logger, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.Event) supervisorrunner.SupervisorRunner
	NewSessionStore     func() sessionstore.SessionStore

	sr supervisorrunner.SupervisorRunner
	ss sessionstore.SessionStore

	ctrlReadyCh  chan struct{}
	closeReqCh   chan error
	closingCh    chan error
	closedCh     chan struct{}
	shuttingDown bool

	eventsCh chan supervisorrunner.Event

	rpcReadyCh chan error
	rpcDoneCh  chan error
}

// NewSupervisorController wires the manager and the shared event channel from sessions.
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
		NewSessionStore:     sessionstore.NewSessionStoreExec,
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
func (s *Controller) Run(spec *api.SupervisorSpec) error {
	s.logger.Info("controller loop started", "spec_kind", spec.Kind, "run_path", spec.RunPath)
	defer s.logger.Info("controller loop stopped", "run_path", spec.RunPath)

	s.sr = s.NewSupervisorRunner(s.ctx, s.logger, spec, s.eventsCh)
	s.ss = s.NewSessionStore()

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

	var session *api.SupervisedSession

	switch spec.Kind {
	case api.RunNewSession:
		s.logger.Info("creating new session", "spec_id", spec.ID, "spec_name", spec.Name)
		var errCreate error
		session, errCreate = s.createRunNewSession(spec)
		if errCreate != nil {
			s.logger.Error("failed to create new session", "error", errCreate)
			if errC := s.Close(errCreate); errC != nil {
				s.logger.Error("error during Close after new session failure", "error", errC)
				errCreate = fmt.Errorf("%w: %w: %w", errCreate, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return errCreate
		}

		if errStart := s.sr.StartSessionCmd(session); errStart != nil {
			s.logger.Error("failed to start session command", "error", errStart)
			if errC := s.Close(errStart); errC != nil {
				s.logger.Error("error during Close after session command failure", "error", errC)
				errStart = fmt.Errorf("%w: %w: %w", errStart, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return fmt.Errorf("%w: %w", errdefs.ErrStartSessionCmd, errStart)
		}
	case api.AttachToSession:
		if spec.SessionSpec == nil || (spec.SessionSpec.ID == "" && spec.SessionSpec.Name == "") {
			s.logger.Error("no session ID or Name provided for attach")
			errAttachSpec := errors.New("no session ID or Name provided for attach")
			if errC := s.Close(errAttachSpec); errC != nil {
				s.logger.Error("error during Close after attach spec failure", "error", errC)
				errAttachSpec = fmt.Errorf("%w: %w: %w", errAttachSpec, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return errAttachSpec
		}
		s.logger.Info(
			"attaching to existing session",
			"attach_id",
			spec.SessionSpec.ID,
			"attach_name",
			spec.SessionSpec.Name,
		)
		var errAttach error
		session, errAttach = s.createAttachSession(spec)
		if errAttach != nil {
			s.logger.Error("failed to create attach session", "error", errAttach)
			if errC := s.Close(errAttach); errC != nil {
				s.logger.Error("error during Close after attach failure", "error", errC)
				errAttach = fmt.Errorf("%w: %w: %w", errAttach, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return fmt.Errorf("%w: %w", errdefs.ErrAttach, errAttach)
		}

	default:
		s.logger.Error("invalid supervisor kind", "kind", spec.Kind)
		errReturn := errdefs.ErrSupervisorKind
		if errC := s.Close(errdefs.ErrSupervisorKind); errC != nil {
			s.logger.Error("error during Close after invalid kind", "error", errC)
			errReturn = fmt.Errorf("%w: %w: %w", errReturn, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		return errReturn
	}

	if errAttach := s.sr.Attach(session); errAttach != nil {
		s.logger.Error("failed to attach to session", "error", errAttach)
		if errC := s.Close(errAttach); errC != nil {
			s.logger.Error("error during Close after attach failure", "error", errC)
			errAttach = fmt.Errorf("%w: %w: %w", errAttach, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		return errAttach
	}

	s.logger.Info("controller ready, entering main event loop")
	close(s.ctrlReadyCh)

	go func() {
		errC := <-s.closingCh
		s.shuttingDown = true
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

func (s *Controller) createAttachSession(spec *api.SupervisorSpec) (*api.SupervisedSession, error) {
	var metadata *api.SessionMetadata
	if spec.SessionSpec.ID != "" {
		s.logger.Debug("resolving terminal by id", "run_path", spec.RunPath, "attach_id", string(spec.SessionSpec.ID))
		var err error
		metadata, err = discovery.FindTerminalByID(s.ctx, s.logger, spec.RunPath, string(spec.SessionSpec.ID))
		if err != nil {
			return nil, errors.New("could not find session by ID")
		}
	} else if spec.SessionSpec.Name != "" {
		s.logger.Debug("resolving terminal by name", "run_path", spec.RunPath, "attach_name", spec.SessionSpec.Name)
		var err error
		metadata, err = discovery.FindTerminalByName(s.ctx, s.logger, spec.RunPath, spec.SessionSpec.Name)
		if err != nil {
			return nil, errors.New("could not find session by Name")
		}
	}

	if metadata == nil {
		return nil, errors.New("no session metadata found to attach")
	}

	session := sessionstore.NewSupervisedSession(&metadata.Spec)
	if err := s.ss.Add(session); err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrSessionStore, err)
	}
	return session, nil
}

func (s *Controller) createRunNewSession(spec *api.SupervisorSpec) (*api.SupervisedSession, error) {
	if spec.SessionSpec == nil {
		return nil, errors.New("no session spec found")
	}

	args := []string{
		"run", "-",
	}

	session := sessionstore.NewSupervisedSession(spec.SessionSpec)

	execPath, errExe := os.Executable()
	if errExe != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrStartCmd, errExe)
	}
	session.Command = execPath
	session.CommandArgs = args

	if errNew := s.ss.Add(session); errNew != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrSessionStore, errNew)
	}

	return session, nil
}

/* ---------- Event handlers ---------- */

func (s *Controller) handleEvent(ev supervisorrunner.Event) {
	switch ev.Type {
	case supervisorrunner.EvCmdExited:
		s.logger.InfoContext(s.ctx, "session EvCmdExited", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)

	case supervisorrunner.EvError:
		s.logger.ErrorContext(s.ctx, "session EvError", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)
	case supervisorrunner.EvDetach:
		s.logger.InfoContext(s.ctx, "session EvDetach", "id", ev.ID)
		err := s.Detach()
		if err != nil {
			s.logger.ErrorContext(s.ctx, "failed to detach session", "id", ev.ID, "error", err)
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
	if !s.shuttingDown {
		s.logger.Info("initiating shutdown sequence", "reason", reason)
		// Set closing reason
		s.closingCh <- reason

		// Notify session runner to close all sessions
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
	// Request detach to session
	if err := s.sr.Detach(); err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrDetachSession, err)
	}
	return nil
}
