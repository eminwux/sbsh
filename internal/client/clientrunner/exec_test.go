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
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/client/clientrpc"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestNewClientRunnerExec_DefaultsAndID verifies the constructor normalises a
// sparse ClientDoc (APIVersion/Kind/Annotations) and that ID() reflects the
// spec ID, while the default stdio falls back to the process handles.
func TestNewClientRunnerExec_DefaultsAndID(t *testing.T) {
	doc := &api.ClientDoc{Spec: api.ClientSpec{ID: api.ID("client-xyz")}}

	cr := NewClientRunnerExec(context.Background(), testLogger(), doc, make(chan Event, 1))

	if got := cr.ID(); got != api.ID("client-xyz") {
		t.Fatalf("ID() = %q; want %q", got, "client-xyz")
	}

	ex, ok := cr.(*Exec)
	if !ok {
		t.Fatalf("NewClientRunnerExec returned %T; want *Exec", cr)
	}
	if ex.metadata.APIVersion != api.APIVersionV1Beta1 {
		t.Errorf("APIVersion = %q; want %q", ex.metadata.APIVersion, api.APIVersionV1Beta1)
	}
	if ex.metadata.Kind != api.KindClient {
		t.Errorf("Kind = %q; want %q", ex.metadata.Kind, api.KindClient)
	}
	if ex.metadata.Metadata.Annotations == nil {
		t.Errorf("Annotations not initialised")
	}
	if ex.stdin == nil || ex.stdout == nil || ex.stderr == nil {
		t.Errorf("stdio handles not defaulted: in=%v out=%v err=%v", ex.stdin, ex.stdout, ex.stderr)
	}
}

// TestNewClientRunnerExecWithIO_CustomHandles verifies caller-supplied stdio
// handles are wired through and preset doc fields are preserved.
func TestNewClientRunnerExecWithIO_CustomHandles(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Annotations: map[string]string{"k": "v"}},
		Spec:       api.ClientSpec{ID: api.ID("c-1")},
	}

	cr := NewClientRunnerExecWithIO(context.Background(), testLogger(), doc, make(chan Event, 1), r, w, w)
	ex := cr.(*Exec)

	if ex.stdin != r || ex.stdout != w || ex.stderr != w {
		t.Fatalf("custom stdio not wired through")
	}
	if ex.metadata.Metadata.Annotations["k"] != "v" {
		t.Fatalf("preset annotations clobbered: %v", ex.metadata.Metadata.Annotations)
	}
}

// TestTrySendEvent covers both the buffered-delivery path and the
// drop-on-full path (the default branch of the non-blocking select).
func TestTrySendEvent(t *testing.T) {
	logger := testLogger()

	ch := make(chan Event, 1)
	ev := Event{ID: api.ID("t1"), Type: EvDetach, When: time.Now()}
	trySendEvent(logger, ch, ev)
	select {
	case got := <-ch:
		if got.ID != ev.ID || got.Type != ev.Type {
			t.Fatalf("event mismatch: got %+v want %+v", got, ev)
		}
	default:
		t.Fatal("expected buffered event to be delivered")
	}

	// Channel already full -> send must not block and must drop silently.
	full := make(chan Event, 1)
	full <- Event{ID: api.ID("pre")}
	trySendEvent(logger, full, Event{ID: api.ID("dropped"), Type: EvError, Err: errors.New("x")})
	if got := <-full; got.ID != api.ID("pre") {
		t.Fatalf("full channel: got %q; want the pre-existing event", got.ID)
	}
}

// TestTestDouble_DefaultsReturnErrFuncNotSet exercises the ClientRunner test
// double: every stub returns ErrFuncNotSet (or a zero value) until its Func
// field is set, and the last-call trackers capture arguments.
func TestTestDouble_DefaultsReturnErrFuncNotSet(t *testing.T) {
	td := NewClientRunnerTest(context.Background(), nil)

	if err := td.OpenSocketCtrl(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("OpenSocketCtrl default = %v; want ErrFuncNotSet", err)
	}
	if err := td.Attach(nil); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Attach default = %v; want ErrFuncNotSet", err)
	}
	if err := td.CreateMetadata(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("CreateMetadata default = %v; want ErrFuncNotSet", err)
	}
	if err := td.Detach(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Detach default = %v; want ErrFuncNotSet", err)
	}
	if err := td.StartTerminalCmd(nil); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("StartTerminalCmd default = %v; want ErrFuncNotSet", err)
	}
	if _, err := td.Metadata(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Metadata default = %v; want ErrFuncNotSet", err)
	}
	if _, err := td.State(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("State default = %v; want ErrFuncNotSet", err)
	}
	if got := td.ID(); got != "" {
		t.Errorf("ID default = %q; want empty", got)
	}

	// Close and Resize record their args even with no Func set.
	reason := errors.New("bye")
	if err := td.Close(reason); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Close default = %v; want ErrFuncNotSet", err)
	}
	if !errors.Is(td.LastReason, reason) {
		t.Errorf("Close did not record LastReason: %v", td.LastReason)
	}
	td.Resize(api.ResizeArgs{Cols: 80, Rows: 24})
	if td.LastResize.Cols != 80 || td.LastResize.Rows != 24 {
		t.Errorf("Resize did not record LastResize: %+v", td.LastResize)
	}
}

// TestTestDouble_StubsAndTrackers verifies the stub functions are invoked and
// StartServer captures ctx/controller on the trackers.
func TestTestDouble_StubsAndTrackers(t *testing.T) {
	called := map[string]bool{}
	td := &Test{
		OpenSocketCtrlFunc:   func() error { called["open"] = true; return nil },
		IDFunc:               func() api.ID { return api.ID("stub-id") },
		CloseFunc:            func(error) error { called["close"] = true; return nil },
		ResizeFunc:           func(api.ResizeArgs) { called["resize"] = true },
		AttachFunc:           func(*api.AttachedTerminal) error { called["attach"] = true; return nil },
		CreateMetadataFunc:   func() error { called["create"] = true; return nil },
		DetachFunc:           func() error { called["detach"] = true; return nil },
		StartTerminalCmdFunc: func(*api.AttachedTerminal) error { called["start"] = true; return nil },
		MetadataFunc:         func() (*api.ClientDoc, error) { return &api.ClientDoc{}, nil },
		StateFunc: func() (*api.ClientStatusMode, error) {
			s := api.ClientReady
			return &s, nil
		},
		StartServerFunc: func(context.Context, *clientrpc.ClientControllerRPC, chan error, chan error) {},
	}

	_ = td.OpenSocketCtrl()
	_ = td.Close(nil)
	td.Resize(api.ResizeArgs{})
	_ = td.Attach(nil)
	_ = td.CreateMetadata()
	_ = td.Detach()
	_ = td.StartTerminalCmd(nil)
	if _, err := td.Metadata(); err != nil {
		t.Fatalf("Metadata stub: %v", err)
	}
	if _, err := td.State(); err != nil {
		t.Fatalf("State stub: %v", err)
	}
	if got := td.ID(); got != api.ID("stub-id") {
		t.Fatalf("ID stub = %q; want stub-id", got)
	}
	for _, k := range []string{"open", "close", "resize", "attach", "create", "detach", "start"} {
		if !called[k] {
			t.Errorf("stub %q was not invoked", k)
		}
	}

	ctx := context.WithValue(context.Background(), ctxTestKey{}, "v")
	td.StartServer(ctx, nil, nil, nil)
	if td.LastCtx == nil || td.LastCtx.Value(ctxTestKey{}) != "v" {
		t.Errorf("StartServer did not record LastCtx")
	}
}

type ctxTestKey struct{}
