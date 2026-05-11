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

// Package server exposes the sbsh terminal RPC server as a reusable
// facade so out-of-tree binaries can serve the same wire protocol
// pkg/attach consumes. The facade hides the multi-subscriber state
// machine, JSON-RPC codec, SCM_RIGHTS FD passing, PID-1 reaper, and
// signal forwarder behind a minimal API whose inputs are an existing
// public api.TerminalSpec and a caller-bound net.Listener.
//
// Typical use case is a process that needs to bind the control socket
// before paying the fork+exec cost — claim the inode, signal readiness,
// then call Serve so the PTY spawns over the already-listening socket.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

// Server is a single-use terminal server. Construct one with New, then
// call Serve. Stop initiates graceful shutdown from another goroutine.
type Server struct {
	spec   *api.TerminalSpec
	logger *slog.Logger

	mu          sync.Mutex
	serveCalled bool
	runner      terminalrunner.TerminalRunner

	stopOnce sync.Once
	stopCh   chan error
}

// New constructs a server bound to spec. The PTY is not spawned here —
// the wrapped child is forked on Serve so a caller can listen on the
// control socket and signal readiness first.
func New(spec *api.TerminalSpec, logger *slog.Logger) (*Server, error) {
	if spec == nil {
		return nil, errors.New("server: nil TerminalSpec")
	}
	if len(spec.Command) == 0 {
		return nil, errors.New("server: empty Command in TerminalSpec")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{
		spec:   spec,
		logger: logger,
		stopCh: make(chan error, 1),
	}, nil
}

// Serve accepts on listener and dispatches the TerminalController
// service surface. Blocks until ctx is cancelled, the wrapped child
// exits, Stop is called, or the RPC server fails. Returns the
// terminating cause. The listener is closed by the underlying runner
// during shutdown — the caller transfers ownership for the lifetime of
// Serve.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return errors.New("server: nil listener")
	}

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	runner, eventsCh, rpcDoneCh, err := s.bringUp(sctx, listener)
	if err != nil {
		return err
	}

	cause := s.runLoop(sctx, eventsCh, rpcDoneCh)
	_ = runner.Close(cause)
	return cause
}

// bringUp performs the one-shot pre-Serve dance: claim the
// serveCalled latch, construct the runner, hand it the listener,
// write the metadata file, kick off the RPC accept loop, wait for it
// to bind, spawn the PTY, and run the SetupShell + OnInitShell
// preroll. Returns the channels Serve needs for its event loop or an
// error if any step fails (in which case the runner is closed and
// the error is wrapped with the failing stage).
func (s *Server) bringUp(
	ctx context.Context,
	listener net.Listener,
) (terminalrunner.TerminalRunner, <-chan terminalrunner.Event, <-chan error, error) {
	s.mu.Lock()
	if s.serveCalled {
		s.mu.Unlock()
		return nil, nil, nil, errors.New("server: Serve already called")
	}
	s.serveCalled = true

	eventsCh := make(chan terminalrunner.Event, eventBufferSize)
	rpcReadyCh := make(chan error, 1)
	rpcDoneCh := make(chan error, 1)

	runner := terminalrunner.NewTerminalRunnerExec(ctx, s.logger, s.spec)
	if err := runner.UseListener(listener); err != nil {
		s.mu.Unlock()
		_ = runner.Close(err)
		return nil, nil, nil, fmt.Errorf("server: UseListener: %w", err)
	}
	s.runner = runner
	s.mu.Unlock()

	if err := runner.CreateMetadata(); err != nil {
		_ = runner.Close(err)
		return nil, nil, nil, fmt.Errorf("server: CreateMetadata: %w", err)
	}

	svc := &terminalrpc.TerminalControllerRPC{Core: &rpcAdapter{srv: s}}
	go runner.StartServer(ctx, svc, rpcReadyCh, rpcDoneCh)

	if err := <-rpcReadyCh; err != nil {
		_ = runner.Close(err)
		return nil, nil, nil, fmt.Errorf("server: StartServer: %w", err)
	}

	if err := runner.StartTerminal(eventsCh); err != nil {
		_ = runner.Close(err)
		return nil, nil, nil, fmt.Errorf("server: StartTerminal: %w", err)
	}

	if err := runner.SetupShell(); err != nil {
		_ = runner.Close(err)
		return nil, nil, nil, fmt.Errorf("server: SetupShell: %w", err)
	}

	if err := runner.OnInitShell(); err != nil {
		_ = runner.Close(err)
		return nil, nil, nil, fmt.Errorf("server: OnInitShell: %w", err)
	}

	return runner, eventsCh, rpcDoneCh, nil
}

// runLoop blocks on the first terminating signal — ctx cancel,
// terminal exit/error event, RPC server exit, or a Stop call — and
// returns the cause to be passed to runner.Close.
func (s *Server) runLoop(
	ctx context.Context,
	eventsCh <-chan terminalrunner.Event,
	rpcDoneCh <-chan error,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-eventsCh:
			if ev.Type == terminalrunner.EvError || ev.Type == terminalrunner.EvCmdExited {
				if ev.Err != nil {
					return ev.Err
				}
				return errors.New("server: terminal exited")
			}
		case err := <-rpcDoneCh:
			if err != nil {
				return err
			}
			return errors.New("server: rpc server exited")
		case err := <-s.stopCh:
			if err != nil {
				return err
			}
			return errors.New("server: Stop called")
		}
	}
}

// Stop initiates graceful shutdown. Safe to call from another
// goroutine while Serve blocks. Returns immediately; the actual
// SIGTERM → grace → SIGKILL sequence runs on the Serve goroutine and
// the Serve call returns once it completes. Multiple calls collapse
// to the first.
func (s *Server) Stop(reason error) error {
	s.stopOnce.Do(func() {
		if reason == nil {
			reason = errors.New("server: Stop called")
		}
		select {
		case s.stopCh <- reason:
		default:
		}
	})
	return nil
}

// Metadata returns the same TerminalDoc the RPC Metadata method
// returns. Errors if called before Serve initializes the runner.
func (s *Server) Metadata() (*api.TerminalDoc, error) {
	r := s.getRunner()
	if r == nil {
		return nil, errors.New("server: not started")
	}
	return r.Metadata()
}

func (s *Server) getRunner() terminalrunner.TerminalRunner {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runner
}

// eventBufferSize matches the controller-side buffer so a momentarily
// busy Serve loop never stalls the PTY-reader goroutine.
const eventBufferSize = 32
