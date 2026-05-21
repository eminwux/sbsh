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

package client

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/client/clientrpc"
	"github.com/eminwux/sbsh/internal/client/clientrunner"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func coverageLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// Test_CreateAttachTerminal_NoSpecReturnsNotFound covers the branch where the
// terminal spec carries no SocketFile, ID, or Name, so resolution yields the
// metadata-not-found sentinel.
func Test_CreateAttachTerminal_NoSpecReturnsNotFound(t *testing.T) {
	sc := NewClientController(context.Background(), coverageLogger()).(*Controller)

	doc := &api.ClientDoc{Spec: api.ClientSpec{TerminalSpec: &api.TerminalSpec{}}}
	_, err := sc.createAttachTerminal(doc)
	if !errors.Is(err, errdefs.ErrTerminalMetadataNotFound) {
		t.Fatalf("createAttachTerminal err = %v; want ErrTerminalMetadataNotFound", err)
	}
}

// Test_CreateAttachTerminal_ByIDNotFound covers the resolve-by-ID failure
// branch when no terminal metadata exists under the run path.
func Test_CreateAttachTerminal_ByIDNotFound(t *testing.T) {
	sc := NewClientController(context.Background(), coverageLogger()).(*Controller)

	doc := &api.ClientDoc{
		Spec: api.ClientSpec{
			RunPath:      t.TempDir(),
			TerminalSpec: &api.TerminalSpec{ID: api.ID("missing-id")},
		},
	}
	_, err := sc.createAttachTerminal(doc)
	if !errors.Is(err, errdefs.ErrTerminalNotFoundByID) {
		t.Fatalf("createAttachTerminal err = %v; want ErrTerminalNotFoundByID", err)
	}
}

// Test_CreateAttachTerminal_ByNameNotFound covers the resolve-by-name failure
// branch.
func Test_CreateAttachTerminal_ByNameNotFound(t *testing.T) {
	sc := NewClientController(context.Background(), coverageLogger()).(*Controller)

	doc := &api.ClientDoc{
		Spec: api.ClientSpec{
			RunPath:      t.TempDir(),
			TerminalSpec: &api.TerminalSpec{Name: "missing-name"},
		},
	}
	_, err := sc.createAttachTerminal(doc)
	if !errors.Is(err, errdefs.ErrTerminalNotFoundByName) {
		t.Fatalf("createAttachTerminal err = %v; want ErrTerminalNotFoundByName", err)
	}
}

// Test_Run_AttachToTerminalLifecycle drives Run end-to-end through the
// AttachToTerminal mode (resolving the terminal by SocketFile), reaching the
// main event loop and then exiting via an EvCmdExited-triggered close. This
// covers the attach-mode branch of the orchestration loop the other Run tests
// (RunNewTerminal mode) leave uncovered.
func Test_Run_AttachToTerminalLifecycle(t *testing.T) {
	sc := NewClientController(context.Background(), coverageLogger()).(*Controller)

	closed := make(chan struct{}, 1)
	sc.NewClientRunner = func(ctx context.Context, logger *slog.Logger, _ *api.ClientDoc, _ chan<- clientrunner.Event) clientrunner.ClientRunner {
		return &clientrunner.Test{
			Ctx:                ctx,
			Logger:             logger,
			CreateMetadataFunc: func() error { return nil },
			OpenSocketCtrlFunc: func() error { return nil },
			StartServerFunc: func(_ context.Context, _ *clientrpc.ClientControllerRPC, readyCh chan error, _ chan error) {
				select {
				case readyCh <- nil:
				default:
				}
			},
			AttachFunc: func(_ *api.AttachedTerminal) error { return nil },
			IDFunc:     func() api.ID { return "term-attach" },
			CloseFunc: func(_ error) error {
				select {
				case closed <- struct{}{}:
				default:
				}
				return nil
			},
		}
	}

	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Annotations: map[string]string{}},
		Spec: api.ClientSpec{
			ID:           api.ID("client-attach"),
			RunPath:      t.TempDir(),
			ClientMode:   api.AttachToTerminal,
			TerminalSpec: &api.TerminalSpec{SocketFile: "/tmp/sbsh-test/attach.sock"},
		},
	}

	exitCh := make(chan error, 1)
	go func() { exitCh <- sc.Run(doc) }()

	<-sc.ctrlReadyCh
	sc.eventsCh <- clientrunner.Event{ID: api.ID("term-attach"), Type: clientrunner.EvCmdExited, When: time.Now()}

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("close was not triggered after EvCmdExited in attach mode")
	}
	<-exitCh
}

// Test_ClassifySessionEnd covers both the peer-closed and detach-requested
// sentinels, including the nil-cause path that returns the bare sentinel.
func Test_ClassifySessionEnd(t *testing.T) {
	sc := NewClientController(context.Background(), coverageLogger()).(*Controller)

	if err := sc.classifySessionEnd(nil); !errors.Is(err, errdefs.ErrPeerClosed) {
		t.Fatalf("classifySessionEnd(nil) = %v; want ErrPeerClosed", err)
	}
	if err := sc.classifySessionEnd(errors.New("hangup")); !errors.Is(err, errdefs.ErrPeerClosed) {
		t.Fatalf("classifySessionEnd(cause) = %v; want wrapped ErrPeerClosed", err)
	}

	sc.detachRequested.Store(true)
	if err := sc.classifySessionEnd(nil); !errors.Is(err, errdefs.ErrClientDetached) {
		t.Fatalf("classifySessionEnd(nil) after detach = %v; want ErrClientDetached", err)
	}
}

// Test_ControllerTestDouble_Defaults verifies the ControllerTest double
// returns ErrFuncNotSet from every stub until a Func field is set.
func Test_ControllerTestDouble_Defaults(t *testing.T) {
	td := &ControllerTest{}

	checks := map[string]func() error{
		"Run":       func() error { return td.Run(nil) },
		"WaitReady": func() error { return td.WaitReady() },
		"Start":     func() error { return td.Start() },
		"Close":     func() error { return td.Close(nil) },
		"WaitClose": func() error { return td.WaitClose() },
		"Detach":    func() error { return td.Detach() },
		"Stop":      func() error { return td.Stop(nil) },
		"Ping":      func() error { _, err := td.Ping(nil); return err },
		"Metadata":  func() error { _, err := td.Metadata(); return err },
		"State":     func() error { _, err := td.State(); return err },
	}
	for name, fn := range checks {
		if err := fn(); !errors.Is(err, errdefs.ErrFuncNotSet) {
			t.Errorf("%s default = %v; want ErrFuncNotSet", name, err)
		}
	}
}

// Test_ControllerTestDouble_StubsInvoked verifies NewClientControllerTest wires
// succeeding defaults for Run/WaitReady/Start and that the remaining stubs are
// invoked when their Func fields are set.
func Test_ControllerTestDouble_StubsInvoked(t *testing.T) {
	td := NewClientControllerTest()
	if err := td.Run(&api.ClientDoc{}); err != nil {
		t.Errorf("Run default = %v; want nil", err)
	}
	if err := td.WaitReady(); err != nil {
		t.Errorf("WaitReady default = %v; want nil", err)
	}
	if err := td.Start(); err != nil {
		t.Errorf("Start default = %v; want nil", err)
	}

	state := api.ClientReady
	td.CloseFunc = func(error) error { return nil }
	td.WaitCloseFunc = func() error { return nil }
	td.DetachFunc = func() error { return nil }
	td.StopFunc = func(*api.StopArgs) error { return nil }
	td.PingFunc = func(in *api.PingMessage) (*api.PingMessage, error) { return in, nil }
	td.MetadataFunc = func() (*api.ClientDoc, error) { return &api.ClientDoc{}, nil }
	td.StateFunc = func() (*api.ClientStatusMode, error) { return &state, nil }

	if err := td.Close(nil); err != nil {
		t.Errorf("Close stub: %v", err)
	}
	if err := td.WaitClose(); err != nil {
		t.Errorf("WaitClose stub: %v", err)
	}
	if err := td.Detach(); err != nil {
		t.Errorf("Detach stub: %v", err)
	}
	if err := td.Stop(&api.StopArgs{}); err != nil {
		t.Errorf("Stop stub: %v", err)
	}
	if _, err := td.Ping(&api.PingMessage{Message: "PING"}); err != nil {
		t.Errorf("Ping stub: %v", err)
	}
	if _, err := td.Metadata(); err != nil {
		t.Errorf("Metadata stub: %v", err)
	}
	if st, err := td.State(); err != nil || *st != api.ClientReady {
		t.Errorf("State stub = %v, %v; want ClientReady, nil", st, err)
	}
}
