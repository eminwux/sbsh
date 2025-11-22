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

package supervisorrunner

import (
	"context"
	"log/slog"
	"net"
	"sync"

	"github.com/eminwux/sbsh/internal/supervisor/terminalstore"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"golang.org/x/term"
)

type Exec struct {
	id         api.ID
	metadata   api.SupervisorDoc
	metadataMu sync.RWMutex // protects metadata field

	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *slog.Logger

	uiMode        UIMode
	lastTermState *term.State

	events   chan<- Event
	terminal *api.SupervisedTerminal
	Mgr      *terminalstore.Exec

	lnCtrl net.Listener

	terminalClient terminal.Client
	ioConn         net.Conn
}

type UIMode int

const (
	UIBash UIMode = iota
	UISupervisor
	UIExitShell // Saved lastState restore
)

func NewSupervisorRunnerExec(
	ctx context.Context,
	logger *slog.Logger,
	doc *api.SupervisorDoc,
	evCh chan<- Event,
) SupervisorRunner {
	newCtx, cancel := context.WithCancel(ctx)

	// Ensure the doc has the correct structure
	if doc.APIVersion == "" {
		doc.APIVersion = api.APIVersionV1Beta1
	}
	if doc.Kind == "" {
		doc.Kind = api.KindSupervisor
	}
	if doc.Metadata.Annotations == nil {
		doc.Metadata.Annotations = make(map[string]string)
	}

	return &Exec{
		id:       doc.Spec.ID,
		metadata: *doc,

		events:    evCh,
		ctx:       newCtx,
		ctxCancel: cancel,
		logger:    logger,
	}
}
