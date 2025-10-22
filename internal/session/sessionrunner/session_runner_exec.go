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

package sessionrunner

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

type SessionRunnerExec struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *slog.Logger

	// immutable
	id       api.ID
	metadata api.SessionMetadata

	// runtime (owned by Session)
	cmd   *exec.Cmd
	pty   *os.File // master
	state api.SessionState

	gates struct {
		StdinOpen bool
		OutputOn  bool
	}

	// observability
	bytesIn, bytesOut uint64
	lastRead          time.Time

	// signaling
	evCh chan<- SessionRunnerEvent // fan-out to controller (send-only from session)

	lnCtrl net.Listener

	clientsMu sync.RWMutex
	clients   map[api.ID]*ioClient

	closeReqCh chan error
	closedCh   chan struct{}

	ptyPipes *ptyPipes

	closePTY *sync.Once
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

func NewSessionRunnerExec(ctx context.Context, logger *slog.Logger, spec *api.SessionSpec) SessionRunner {
	newCtx, cancel := context.WithCancel(ctx)

	return &SessionRunnerExec{
		id: spec.ID,
		metadata: api.SessionMetadata{
			Spec:   *spec,
			Status: api.SessionStatus{},
		},

		ctx:       newCtx,
		ctxCancel: cancel,
		logger:    logger,

		// runtime (initialized but inactive)
		cmd:   nil,
		pty:   nil,
		state: api.SessionBash, // default logical state before start

		clients: make(map[api.ID]*ioClient),

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

		closeReqCh: make(chan error),
		closedCh:   make(chan struct{}),
		ptyPipes:   &ptyPipes{},
		closePTY:   &sync.Once{},
	}
}

func (sr *SessionRunnerExec) ID() api.ID {
	return sr.id
}
