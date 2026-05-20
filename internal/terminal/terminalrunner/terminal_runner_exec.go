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

package terminalrunner

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/eminwux/sbsh/internal/initmode"
	"github.com/eminwux/sbsh/pkg/api"
)

type Exec struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *slog.Logger

	// immutable
	id         api.ID
	metadata   api.TerminalDoc
	metadataMu sync.RWMutex // protects metadata field

	// runtime (owned by Terminal)
	cmd   *exec.Cmd
	ptmx  *os.File // master
	pts   *os.File // slave
	state api.TerminalState

	// processes holds the supervised processes spawned from a non-empty
	// Spec.Processes (the process-set path; mutually exclusive with the single
	// top-level Command). Empty for the single-child path. Phase 2 spawns and
	// drains each process; multiplexing onto the operator and the
	// all-processes-exited lifecycle ship in later phases.
	processes   []*procState
	processesMu sync.Mutex

	gates struct {
		StdinOpen bool
		OutputOn  bool
	}
	obsMu sync.RWMutex // protects gates, bytesIn, bytesOut, lastRead

	// observability
	bytesIn, bytesOut uint64
	lastRead          time.Time

	// signaling
	evCh chan<- Event // fan-out to controller (send-only from Terminal)

	lnCtrl net.Listener

	clientsMu sync.RWMutex
	clients   map[api.ID]*ioClient

	subsMu      sync.Mutex
	subscribers map[*subscriberWriter]struct{}

	closeReqCh chan error
	closedCh   chan struct{}

	// childDoneCh is closed once by the child-watching goroutine (either
	// os/exec.Wait or the PID-1 reaper) once the tracked child has exited.
	// Close() uses it as the "is the child dead yet" signal during graceful
	// shutdown so it can decide when to escalate SIGTERM -> SIGKILL.
	childDoneCh   chan struct{}
	childDoneOnce *sync.Once

	ptyPipes   *ptyPipes
	ptyPipesMu sync.RWMutex // protects ptyPipes field (reads after initialization)

	closePTY *sync.Once

	// captureFile is the *os.File backing the always-on capture writer in
	// multiOutW. The runner owns the fd from startPty onward; Close closes
	// it via closeCapture so repeated New→Start→Close cycles in-process do
	// not leak fds. See #229.
	captureFile  *os.File
	closeCapture *sync.Once

	// closeClosedCh guards close(closedCh) so a second Close call does not
	// panic on a channel that is already closed. Matches the per-resource
	// once pattern used for closePTY/closeCapture. See #242.
	closeClosedCh *sync.Once

	// initMode is captured at construction time so tests can toggle
	// initmode.Enable before/after NewTerminalRunnerExec without surprising
	// the already-running goroutines.
	initMode bool

	// reaper is the PID-1 zombie reaper, non-nil only when initMode is true.
	reaper *reaper

	// stopSignalForwarder tears down the PID-1 signal forwarder. nil when
	// initMode is false.
	stopSignalForwarder func()
}

type ptyPipes struct {
	pipeInR   *os.File
	pipeInW   *os.File
	multiOutW *DynamicMultiWriter
}

type ioClient struct {
	id        *api.ID
	conn      net.Conn
	outWriter *subscriberWriter
}

func NewTerminalRunnerExec(ctx context.Context, logger *slog.Logger, spec *api.TerminalSpec) TerminalRunner {
	newCtx, cancel := context.WithCancel(ctx)

	inInit := initmode.IsInit()
	var rp *reaper
	if inInit {
		rp = newReaper(logger)
		rp.Start()
	}

	return &Exec{
		id: spec.ID,
		metadata: api.TerminalDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindTerminal,
			Metadata: api.TerminalMetadata{
				Name:        spec.Name,
				Labels:      spec.Labels,
				Annotations: make(map[string]string),
			},
			Spec:   *spec,
			Status: api.TerminalStatus{},
		},

		ctx:       newCtx,
		ctxCancel: cancel,
		logger:    logger,

		// runtime (initialized but inactive)
		cmd:   nil,
		ptmx:  nil,
		state: api.TerminalBash, // default logical state before start

		clients:     make(map[api.ID]*ioClient),
		subscribers: make(map[*subscriberWriter]struct{}),

		gates: struct {
			StdinOpen bool
			OutputOn  bool
		}{
			StdinOpen: true, // allow stdin by default once started
			OutputOn:  true, // render PTY output by default
		},

		// observability (zeroed; will be updated when running)
		bytesIn:  0,
		bytesOut: 0,

		// signaling (set in Start)
		evCh: nil, // assigned in Start(...)

		closeReqCh:    make(chan error),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{},
		closePTY:      &sync.Once{},
		closeCapture:  &sync.Once{},
		closeClosedCh: &sync.Once{},

		initMode: inInit,
		reaper:   rp,
	}
}

// markChildDone records that the child has exited. Safe to call from any
// goroutine; subsequent calls are no-ops. Returning the bool lets the caller
// know whether this call was the first.
func (sr *Exec) markChildDone() bool {
	fired := false
	sr.childDoneOnce.Do(func() {
		close(sr.childDoneCh)
		fired = true
	})
	return fired
}

// childDone returns true if the child has exited (non-blocking).
func (sr *Exec) childDone() bool {
	select {
	case <-sr.childDoneCh:
		return true
	default:
		return false
	}
}

func (sr *Exec) ID() api.ID {
	return sr.id
}

// UseListener installs an externally-bound control-socket listener so
// that OpenSocketCtrl is not the only path that can prime the accept
// loop. Intended for the public pkg/terminal/server facade where the
// caller owns the inode (e.g. kuketty claims it before fork+exec).
// Must be called before StartServer.
//
// Spec.SocketMode and Spec.SocketGID are applied to the inode resolved
// from ln.Addr() so the on-disk permissions match what the
// OpenSocketCtrl path produces. The listener is stored before the
// chmod/chown so a failure here leaves it owned by the runner — the
// caller can rely on runner.Close to tear it down.
func (sr *Exec) UseListener(ln net.Listener) error {
	sr.lnCtrl = ln

	sr.metadataMu.RLock()
	socketMode := sr.metadata.Spec.SocketMode
	var socketGID *int
	if g := sr.metadata.Spec.SocketGID; g != nil {
		gv := *g
		socketGID = &gv
	}
	sr.metadataMu.RUnlock()

	return sr.applySocketPerms(ln.Addr().String(), socketMode, socketGID)
}
