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

package clientrunner

import (
	"context"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/eminwux/sbsh/internal/client/terminalstore"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"golang.org/x/term"
)

type Exec struct {
	id         api.ID
	metadata   api.ClientDoc
	metadataMu sync.RWMutex // protects metadata field

	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *slog.Logger

	uiMode        UIMode
	lastTermState *term.State

	// stdin/stdout/stderr are the user-facing terminal handles. They
	// default to os.Stdin/Stdout/Stderr but can be overridden by
	// embedders via NewClientRunnerExecWithIO so the attach loop drives
	// caller-supplied file handles instead of the process's own tty.
	stdin  *os.File
	stdout *os.File
	stderr *os.File

	events   chan<- Event
	terminal *api.AttachedTerminal
	Mgr      *terminalstore.Exec

	lnCtrl net.Listener

	terminalClient terminal.Client
	ioConn         net.Conn
}

type UIMode int

const (
	UIBash UIMode = iota
	UIClient
	UIExitShell // Saved lastState restore
)

func NewClientRunnerExec(
	ctx context.Context,
	logger *slog.Logger,
	doc *api.ClientDoc,
	evCh chan<- Event,
) ClientRunner {
	return NewClientRunnerExecWithIO(ctx, logger, doc, evCh, nil, nil, nil)
}

// NewClientRunnerExecWithIO is like NewClientRunnerExec but lets the
// caller plug in custom stdin/stdout/stderr handles instead of the
// process's own. A nil handle falls back to the corresponding os.Std*.
func NewClientRunnerExecWithIO(
	ctx context.Context,
	logger *slog.Logger,
	doc *api.ClientDoc,
	evCh chan<- Event,
	stdin, stdout, stderr *os.File,
) ClientRunner {
	newCtx, cancel := context.WithCancel(ctx)

	// Ensure the doc has the correct structure
	if doc.APIVersion == "" {
		doc.APIVersion = api.APIVersionV1Beta1
	}
	if doc.Kind == "" {
		doc.Kind = api.KindClient
	}
	if doc.Metadata.Annotations == nil {
		doc.Metadata.Annotations = make(map[string]string)
	}

	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	return &Exec{
		id:       doc.Spec.ID,
		metadata: *doc,

		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,

		events:    evCh,
		ctx:       newCtx,
		ctxCancel: cancel,
		logger:    logger,
	}
}
