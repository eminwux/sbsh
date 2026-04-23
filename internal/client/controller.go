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

package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/eminwux/sbsh/internal/client/clientrpc"
	"github.com/eminwux/sbsh/internal/client/clientrunner"
	"github.com/eminwux/sbsh/internal/client/terminalstore"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

/* ---------- Controller ---------- */

// Controller manages the lifecycle of the client.
type Controller struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	logger *slog.Logger

	NewClientRunner  func(ctx context.Context, logger *slog.Logger, doc *api.ClientDoc, evCh chan<- clientrunner.Event) clientrunner.ClientRunner
	NewTerminalStore func() terminalstore.TerminalStore

	sr clientrunner.ClientRunner
	ss terminalstore.TerminalStore

	ctrlReadyCh  chan struct{}
	closeReqCh   chan error
	closingCh    chan error
	closedCh     chan struct{}
	shuttingDown atomic.Bool

	eventsCh chan clientrunner.Event

	rpcReadyCh chan error
	rpcDoneCh  chan error
}

// NewClientController wires the manager and the shared event channel from terminals.
func NewClientController(ctx context.Context, logger *slog.Logger) api.ClientController {
	logger.InfoContext(ctx, "New client controller is being created")
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
		eventsCh:         make(chan clientrunner.Event, 32),
		NewClientRunner:  clientrunner.NewClientRunnerExec,
		NewTerminalStore: terminalstore.NewTerminalStoreExec,
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
func (s *Controller) Run(doc *api.ClientDoc) error {
	clientMode := doc.Spec.ClientMode
	s.logger.Info("controller loop started", "client_mode", clientMode, "run_path", doc.Spec.RunPath)
	defer s.logger.Info("controller loop stopped", "run_path", doc.Spec.RunPath)

	s.sr = s.NewClientRunner(s.ctx, s.logger, doc, s.eventsCh)
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

	rpc := &clientrpc.ClientControllerRPC{Core: s}

	go s.sr.StartServer(s.ctx, rpc, s.rpcReadyCh, s.rpcDoneCh)
	// Wait for startup result
	if errRPC := <-s.rpcReadyCh; errRPC != nil {
		if errC := s.Close(errRPC); errC != nil {
			s.logger.Error("error during Close after RPC server start failure", "error", errC)
			errRPC = fmt.Errorf("%w: %w: %w", errRPC, errdefs.ErrOnClose, errC)
		}
		close(s.ctrlReadyCh)
		s.logger.Error("failed to start client RPC server", "error", errRPC)
		return fmt.Errorf("%w: %w", errdefs.ErrStartRPCServer, errRPC)
	}

	var terminal *api.AttachedTerminal

	switch clientMode {
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
		s.logger.Error("invalid client mode", "mode", clientMode)
		errReturn := errdefs.ErrClientMode
		if errC := s.Close(errdefs.ErrClientMode); errC != nil {
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
				"received client event",
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
			if errClose == nil {
				return nil
			}
			return fmt.Errorf("%w: %w", errdefs.ErrCloseReq, errClose)
		}
	}
}

func (s *Controller) createAttachTerminal(doc *api.ClientDoc) (*api.AttachedTerminal, error) {
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

func (s *Controller) createRunNewTerminal(doc *api.ClientDoc) (*api.AttachedTerminal, error) {
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

func (s *Controller) handleEvent(ev clientrunner.Event) {
	switch ev.Type {
	case clientrunner.EvCmdExited:
		s.logger.InfoContext(s.ctx, "terminal EvCmdExited", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)

	case clientrunner.EvError:
		s.logger.ErrorContext(s.ctx, "terminal EvError", "id", ev.ID, "err", ev.Err)
		s.onClosed(ev.ID, ev.Err)
	case clientrunner.EvDetach:
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

func (s *Controller) Ping(in *api.PingMessage) (*api.PingMessage, error) {
	if in != nil && in.Message == "PING" {
		return &api.PingMessage{Message: "PONG"}, nil
	}
	msg := ""
	if in != nil {
		msg = in.Message
	}
	return &api.PingMessage{}, fmt.Errorf("unexpected ping message: %s", msg)
}

func (s *Controller) Metadata() (*api.ClientDoc, error) {
	if s.sr == nil {
		return nil, errors.New("client runner not initialized")
	}
	return s.sr.Metadata()
}

func (s *Controller) State() (*api.ClientStatusMode, error) {
	if s.sr == nil {
		return nil, errors.New("client runner not initialized")
	}
	return s.sr.State()
}

func (s *Controller) Stop(args *api.StopArgs) error {
	reason := "stop requested"
	if args != nil && args.Reason != "" {
		reason = args.Reason
	}
	s.logger.InfoContext(s.ctx, "Stop RPC invoked", "reason", reason)
	// Close asynchronously so the RPC reply can be written before the
	// server socket is torn down.
	go func() {
		_ = s.Close(fmt.Errorf("stop: %s", reason))
	}()
	return nil
}
