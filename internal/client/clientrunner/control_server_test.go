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
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/client/clientrpc"
	"github.com/eminwux/sbsh/pkg/api"
)

// TestStartServer_ServesThenStopsOnCancel binds a control socket, starts the
// RPC accept loop, dials it to exercise the per-conn ServeCodec goroutine, and
// confirms the loop signals ready=nil and done=nil on context cancel.
func TestStartServer_ServesThenStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runPath := t.TempDir()
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	sr := &Exec{
		id:         api.ID("client-srv"),
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		metadataMu: sync.RWMutex{},
		metadata: api.ClientDoc{
			Spec: api.ClientSpec{ID: api.ID("client-srv"), RunPath: runPath, SockerCtrl: sockPath},
		},
	}

	if err := sr.OpenSocketCtrl(); err != nil {
		t.Fatalf("OpenSocketCtrl: %v", err)
	}

	readyCh := make(chan error, 1)
	doneCh := make(chan error, 1)
	go sr.StartServer(ctx, &clientrpc.ClientControllerRPC{}, readyCh, doneCh)

	select {
	case err := <-readyCh:
		if err != nil {
			t.Fatalf("ready signal = %v; want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer never signalled ready")
	}

	// Confirm the accept loop is live and the state advanced to ClientReady.
	if conn, err := net.DialTimeout("unix", sockPath, time.Second); err == nil {
		_ = conn.Close()
	} else {
		t.Fatalf("dial control socket: %v", err)
	}
	if st, _ := sr.State(); *st != api.ClientReady {
		t.Errorf("state after ready = %v; want ClientReady", *st)
	}

	cancel()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("done signal = %v; want nil on clean cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer never signalled done after cancel")
	}
}
