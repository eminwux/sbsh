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

// SupervisorController manages the lifecycle of the supervisor.
type SupervisorController struct {
	ctx    context.Context
	logger *slog.Logger

	NewSupervisorRunner func(ctx context.Context, logger *slog.Logger, spec *api.SupervisorSpec, evCh chan<- supervisorrunner.SupervisorRunnerEvent) supervisorrunner.SupervisorRunner
	NewSessionStore     func() sessionstore.SessionStore

	sr supervisorrunner.SupervisorRunner
	ss sessionstore.SessionStore

	ctrlReadyCh  chan struct{}
	closeReqCh   chan error
	closingCh    chan error
	closedCh     chan struct{}
	shuttingDown bool

	eventsCh chan supervisorrunner.SupervisorRunnerEvent

	rpcReadyCh chan error
	rpcDoneCh  chan error
}

// NewSupervisorController wires the manager and the shared event channel from sessions.
func NewSupervisorController(ctx context.Context, logger *slog.Logger) api.SupervisorController {
	logger.InfoContext(ctx, "New supervisor controller is being created")

	c := &SupervisorController{
		ctx:         ctx,
		logger:      logger,
		closedCh:    make(chan struct{}),
		closingCh:   make(chan error, 1),
		ctrlReadyCh: make(chan struct{}),
		rpcReadyCh:  make(chan error),
		rpcDoneCh:   make(chan error),
		closeReqCh:  make(chan error, 1),
		//nolint:mnd // event channel buffer size
		eventsCh:            make(chan supervisorrunner.SupervisorRunnerEvent, 32),
		NewSupervisorRunner: supervisorrunner.NewSupervisorRunnerExec,
		NewSessionStore:     sessionstore.NewSessionStoreExec,
	}
	return c
}

func (s *SupervisorController) WaitReady() error {
	select {
	case <-s.ctrlReadyCh:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (s *SupervisorController) Run(spec *api.SupervisorSpec) error {
	s.logger.Info("controller loop started", "spec_kind", spec.Kind, "run_path", spec.RunPath)
	defer s.logger.Info("controller loop stopped", "run_path", spec.RunPath)

	s.sr = s.NewSupervisorRunner(s.ctx, s.logger, spec, s.eventsCh)
	s.ss = s.NewSessionStore()

	err := s.sr.CreateMetadata()
	if err != nil {
		s.logger.Error("failed to write metadata file", "error", err)
		if errC := s.Close(err); errC != nil {
			s.logger.Error("error during Close after metadata failure", "error", errC)
			err = fmt.Errorf("%w: %w :%w", err, errdefs.ErrOnClose, errC)
		}
		return fmt.Errorf("%w: %w", errdefs.ErrWriteMetadata, err)
	}

	err = s.sr.OpenSocketCtrl()
	if err != nil {
		s.logger.Error("failed to open control socket", "error", err)
		if errC := s.Close(err); errC != nil {
			s.logger.Error("error during Close after socket open failure", "error", errC)
			err = fmt.Errorf("%w: %w :%w", errdefs.ErrOnClose, err, errC)
		}
		close(s.ctrlReadyCh)
		return fmt.Errorf("%w: %w", errdefs.ErrOpenSocketCtrl, err)
	}

	rpc := &supervisorrpc.SupervisorControllerRPC{Core: s}

	go s.sr.StartServer(s.ctx, rpc, s.rpcReadyCh, s.rpcDoneCh)
	// Wait for startup result
	if errB := <-s.rpcReadyCh; errB != nil {
		if errC := s.Close(errB); errC != nil {
			s.logger.Error("error during Close after RPC server start failure", "error", errC)
			err = fmt.Errorf("%w: %w: %w", errB, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		s.logger.Error("failed to start supervisor RPC server", "error", errB)
		return fmt.Errorf("%w: %w", errdefs.ErrStartRPCServer, err)
	}

	var session *api.SupervisedSession

	switch spec.Kind {
	case api.RunNewSession:
		s.logger.Info("creating new session", "spec_id", spec.ID, "spec_name", spec.Name)
		session, err = s.CreateRunNewSession(spec)
		if err != nil {
			s.logger.Error("failed to create new session", "error", err)
			if errC := s.Close(err); errC != nil {
				s.logger.Error("error during Close after new session failure", "error", errC)
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return err
		}

		if err := s.sr.StartSessionCmd(session); err != nil {
			s.logger.Error("failed to start session command", "error", err)
			if errC := s.Close(err); errC != nil {
				s.logger.Error("error during Close after session command failure", "error", errC)
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return fmt.Errorf("%w: %w", errdefs.ErrStartSessionCmd, err)
		}
	case api.AttachToSession:
		s.logger.Info("attaching to existing session", "attach_id", spec.AttachID, "attach_name", spec.AttachName)
		session, err = s.CreateAttachSession(spec)
		if err != nil {
			s.logger.Error("failed to attach to session", "error", err)
			if errC := s.Close(err); errC != nil {
				s.logger.Error("error during Close after attach failure", "error", errC)
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
			}
			close(s.ctrlReadyCh)
			return err
		}

	default:
		s.logger.Error("invalid supervisor kind", "kind", spec.Kind)
		if err := s.Close(err); err != nil {
			s.logger.Error("error during Close after invalid kind", "error", err)
			err = fmt.Errorf("%w: %w", errdefs.ErrOnClose, err)
		}
		close(s.ctrlReadyCh)
		return errdefs.ErrSupervisorKind
	}

	if err := s.sr.Attach(session); err != nil {
		s.logger.Error("failed to attach to session", "error", err)
		if err := s.Close(err); err != nil {
			s.logger.Error("error during Close after attach failure", "error", err)
			err = fmt.Errorf("%w: %w", errdefs.ErrOnClose, err)
		}
		close(s.ctrlReadyCh)
		return fmt.Errorf("%w: %w", errdefs.ErrAttach, err)
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
			var err error
			s.logger.Warn("parent context canceled, shutting down controller")
			if errC := s.Close(s.ctx.Err()); errC != nil {
				s.logger.Error("error during Close after context done", "error", errC)
				err = fmt.Errorf("%w: %w", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)

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

		case err := <-s.rpcDoneCh:
			s.logger.Error("rpc server exited", "error", err)
			if errC := s.Close(err); err != nil {
				s.logger.Error("error during Close after rpc server failure", "error", errC)
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %w", errdefs.ErrRPCServerExited, err)

		case err := <-s.closeReqCh:
			s.logger.Warn("close request received", "error", err)
			return fmt.Errorf("%w: %w", errdefs.ErrCloseReq, err)
		}
	}
}

func (s *SupervisorController) CreateAttachSession(spec *api.SupervisorSpec) (*api.SupervisedSession, error) {
	// read from metadata

	var metadata *api.SessionMetadata
	var err error
	if spec.AttachID != "" {
		metadata, err = discovery.FindSessionByID(s.ctx, s.logger, spec.RunPath, string(spec.AttachID))
		if err != nil {
			return nil, errors.New("coult not find session by ID")
		}
	} else if spec.AttachName != "" {
		metadata, err = discovery.FindSessionByName(s.ctx, s.logger, spec.RunPath, spec.AttachName)
		if err != nil {
			return nil, errors.New("coult not find session by Name")
		}
	}

	if metadata == nil {
		return nil, errdefs.ErrAttach
	}

	session := sessionstore.NewSupervisedSession(&metadata.Spec)
	if err := s.ss.Add(session); err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrSessionStore, err)
	}
	return session, nil
}

func (s *SupervisorController) CreateRunNewSession(spec *api.SupervisorSpec) (*api.SupervisedSession, error) {
	// sessionID := naming.RandomID()
	// sessionName := naming.RandomSessionName()

	if spec.SessionSpec == nil {
		return nil, errors.New("no session spec found")
	}
	// args := []string{"run", "--id", sessionID, "--name", sessionName}
	args := []string{
		"run", "--id",
		string(spec.SessionSpec.ID), "--name",
		spec.SessionSpec.Name,
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrStartCmd, err)
	}
	spec.SessionSpec.Command = execPath
	spec.SessionSpec.CommandArgs = args

	// spec.SessionMetadata.Spec.LogFilename = s.runPath + "/sessions/" + string(spec.SessionMetadata.Spec.ID) + "/log"
	// spec.SessionMetadata.Spec.SockerCtrl = s.runPath + "/sessions/" + string(
	// 	spec.SessionMetadata.Spec.ID,
	// ) + "/ctrl.sock"
	// spec.SessionMetadata.Spec.SocketIO = s.runPath + "/sessions/" + string(spec.SessionMetadata.Spec.ID) + "/io.sock"

	session := sessionstore.NewSupervisedSession(spec.SessionSpec)
	if err := s.ss.Add(session); err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrSessionStore, err)
	}
	return session, nil
}

/* ---------- Event handlers ---------- */

func (s *SupervisorController) handleEvent(ev supervisorrunner.SupervisorRunnerEvent) {
	switch ev.Type {
	case supervisorrunner.EvCmdExited:
		s.logger.Info("session EvCmdExited", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)

	case supervisorrunner.EvError:
		s.logger.Error("session EvError", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)
	}
}

func (s *SupervisorController) onClosed(_ api.ID, err error) {
	s.logger.Warn("onClosed called", "err", err)
	s.Close(err)
}

func (s *SupervisorController) Close(reason error) error {
	if !s.shuttingDown {
		s.logger.Info("initiating shutdown sequence", "reason", reason)
		// Set closing reason
		s.closingCh <- reason

		// Notify session runner to close all sessions
		s.sr.Close(reason)

		// Notify Run to exit
		s.closeReqCh <- reason

		// Mark controller as closed
		close(s.closedCh)
	} else {
		s.logger.Info("shutdown sequence already in progress, ignoring duplicate request", "reason", reason)
	}
	return nil
}

func (s *SupervisorController) WaitClose() error {
	select {
	case <-s.closedCh:
		s.logger.Debug("controller has fully exited and resources are released")
		return nil
	}
}

func (s *SupervisorController) Detach() error {
	// Request detach to session
	if err := s.sr.Detach(); err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrDetachSession, err)
	}
	return nil
}
