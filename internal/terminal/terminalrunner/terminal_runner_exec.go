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

	closeReqCh chan error
	closedCh   chan struct{}

	ptyPipes   *ptyPipes
	ptyPipesMu sync.RWMutex // protects ptyPipes field (reads after initialization)

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

func NewTerminalRunnerExec(ctx context.Context, logger *slog.Logger, spec *api.TerminalSpec) TerminalRunner {
	newCtx, cancel := context.WithCancel(ctx)

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

func (sr *Exec) ID() api.ID {
	return sr.id
}
