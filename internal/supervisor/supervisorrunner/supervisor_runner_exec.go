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

	"github.com/eminwux/sbsh/internal/supervisor/sessionstore"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/rpcclient/session"
	"golang.org/x/term"
)

type Exec struct {
	id       api.ID
	metadata api.SupervisorMetadata

	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *slog.Logger

	uiMode        UIMode
	lastTermState *term.State

	events  chan<- Event
	session *api.SupervisedSession
	Mgr     *sessionstore.Exec

	lnCtrl net.Listener

	sessionClient session.Client
	ioConn        net.Conn
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
	spec *api.SupervisorSpec,
	evCh chan<- Event,
) SupervisorRunner {
	newCtx, cancel := context.WithCancel(ctx)

	return &Exec{
		id: spec.ID,
		metadata: api.SupervisorMetadata{
			Spec: *spec,
		},

		events:    evCh,
		ctx:       newCtx,
		ctxCancel: cancel,
		logger:    logger,
	}
}
