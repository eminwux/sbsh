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
	"bytes"
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
)

// newStartServerTestExec returns a minimal Exec wired with ctx, logger, and
// a Unix listener bound to a temp socket. StartServer only touches lnCtrl
// and sr.ctx on the accept path, so the heavier runner state can stay nil.
func newStartServerTestExec(t *testing.T, logger *slog.Logger) (*Exec, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		cancel()
		t.Fatalf("listen unix: %v", err)
	}
	sr := &Exec{
		ctx:           ctx,
		ctxCancel:     cancel,
		logger:        logger,
		id:            "graceful-test",
		lnCtrl:        ln,
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{},
		closePTY:      &sync.Once{},
	}
	cleanup := func() {
		cancel()
		_ = ln.Close()
	}
	return sr, cleanup
}

// TestStartServer_GracefulCtxCancelSendsNil is the regression for #228.
//
// Pre-fix, StartServer sent `errors.New("unknown rpc server error")` on the
// graceful ctx-cancel path, causing consumers (controller.go,
// pkg/terminal/server/server.go) to treat a clean shutdown as an abnormal
// failure and emit a misleading "rpc server has failed" log line.
// Post-fix, doneCh receives `nil` on this path, matching the clientrunner
// sibling at internal/client/clientrunner/control_server.go.
func TestStartServer_GracefulCtxCancelSendsNil(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	sr, cleanup := newStartServerTestExec(t, logger)
	defer cleanup()

	readyCh := make(chan error, 1)
	doneCh := make(chan error, 1)

	svc := &terminalrpc.TerminalControllerRPC{Core: nil}

	go sr.StartServer(sr.ctx, svc, readyCh, doneCh)

	select {
	case err := <-readyCh:
		if err != nil {
			t.Fatalf("StartServer ready returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer did not signal ready within 2s")
	}

	sr.ctxCancel()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("graceful ctx-cancel path delivered non-nil err on doneCh: %v "+
				"(want nil, matching clientrunner sibling)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer did not exit within 2s after ctx cancel")
	}

	if got := logBuf.String(); strings.Contains(got, "rpc server has failed") {
		t.Fatalf("graceful shutdown emitted spurious 'rpc server has failed' log line; got:\n%s", got)
	}
}
