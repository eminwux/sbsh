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
	id       *api.ID
	conn     net.Conn
	pipeOutR *os.File
	pipeOutW *os.File
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
