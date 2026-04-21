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

//go:build unix

package spawn

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

func validAttachDoc(runPath, sockPath string) *api.ClientDoc {
	return &api.ClientDoc{
		Spec: api.ClientSpec{
			RunPath:    runPath,
			SockerCtrl: sockPath,
			ClientMode: api.AttachToTerminal,
			TerminalSpec: &api.TerminalSpec{
				ID: "t-1",
			},
		},
	}
}

func TestNewClient_Validation(t *testing.T) {
	t.Parallel()

	wrongMode := validAttachDoc("/run", "/run/c.sock")
	wrongMode.Spec.ClientMode = api.RunNewTerminal

	noTarget := validAttachDoc("/run", "/run/c.sock")
	noTarget.Spec.TerminalSpec = &api.TerminalSpec{}

	noSocket := validAttachDoc("/run", "")

	tests := []struct {
		name string
		doc  *api.ClientDoc
		opts ClientOptions
		want error
	}{
		{
			name: "missing binary",
			doc:  validAttachDoc("/run", "/run/c.sock"),
			opts: ClientOptions{},
			want: ErrBinaryPathRequired,
		},
		{
			name: "nil doc",
			doc:  nil,
			opts: ClientOptions{BinaryPath: "/bin/true"},
			want: ErrDocRequired,
		},
		{
			name: "wrong client mode",
			doc:  wrongMode,
			opts: ClientOptions{BinaryPath: "/bin/true"},
			want: ErrUnsupportedClientMode,
		},
		{
			name: "missing socket",
			doc:  noSocket,
			opts: ClientOptions{BinaryPath: "/bin/true"},
			want: ErrSocketPathRequired,
		},
		{
			name: "missing attach target",
			doc:  noTarget,
			opts: ClientOptions{BinaryPath: "/bin/true"},
			want: ErrAttachTargetRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewClient(context.Background(), tc.doc, tc.opts)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NewClient err = %v; want %v", err, tc.want)
			}
		})
	}
}

// TestBuildClientAttachArgs_ByID covers the argv-construction rules for
// `sb attach`: --run-path is a root-level flag, --socket + --id are
// attach-level flags, and --disable-detach reflects the inverted
// DetachKeystroke boolean.
func TestBuildClientAttachArgs_ByID(t *testing.T) {
	t.Parallel()
	doc := &api.ClientDoc{
		Spec: api.ClientSpec{
			RunPath:         "/opt/kukeon/sbsh",
			SockerCtrl:      "/opt/kukeon/sbsh/clients/c-1/socket",
			ClientMode:      api.AttachToTerminal,
			DetachKeystroke: true, // do NOT pass --disable-detach
			TerminalSpec:    &api.TerminalSpec{ID: "t-1"},
		},
	}
	got := buildClientAttachArgs(doc, ClientOptions{})
	want := []string{
		"--run-path", "/opt/kukeon/sbsh",
		"attach",
		"--socket", "/opt/kukeon/sbsh/clients/c-1/socket",
		"--id", "t-1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildClientAttachArgs = %q; want %q", got, want)
	}
}

// TestBuildClientAttachArgs_ByName_DisableDetach covers the other leg:
// resolving the terminal positionally by name, with the detach
// keystroke suppressed via --disable-detach.
func TestBuildClientAttachArgs_ByName_DisableDetach(t *testing.T) {
	t.Parallel()
	doc := &api.ClientDoc{
		Spec: api.ClientSpec{
			RunPath:         "/run/sbsh",
			SockerCtrl:      "/run/sbsh/clients/c-2/socket",
			ClientMode:      api.AttachToTerminal,
			DetachKeystroke: false,
			TerminalSpec:    &api.TerminalSpec{Name: "dev"},
		},
	}
	got := buildClientAttachArgs(doc, ClientOptions{ExtraArgs: []string{"--verbose"}})
	want := []string{
		"--run-path", "/run/sbsh",
		"--verbose",
		"attach",
		"--socket", "/run/sbsh/clients/c-2/socket",
		"--disable-detach",
		"dev",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildClientAttachArgs = %q; want %q", got, want)
	}
}

// TestBuildClientAttachArgs_IDWinsOverName locks down the tie-break
// rule: when a TerminalSpec carries both ID and Name, spawn emits
// --id only (no positional name arg). This matches `sb attach`, which
// rejects the ID+name combination in `cmd/sb/attach/attach.go:72-83`
// — spawn never hands it the ambiguous input in the first place.
func TestBuildClientAttachArgs_IDWinsOverName(t *testing.T) {
	t.Parallel()
	doc := &api.ClientDoc{
		Spec: api.ClientSpec{
			RunPath:         "/run/sbsh",
			SockerCtrl:      "/run/sbsh/clients/c-3/socket",
			ClientMode:      api.AttachToTerminal,
			DetachKeystroke: true,
			TerminalSpec:    &api.TerminalSpec{ID: "t-3", Name: "both-set"},
		},
	}
	got := buildClientAttachArgs(doc, ClientOptions{})
	want := []string{
		"--run-path", "/run/sbsh",
		"attach",
		"--socket", "/run/sbsh/clients/c-3/socket",
		"--id", "t-3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildClientAttachArgs = %q; want %q", got, want)
	}
}

// TestWrapProcessExited_NilExitError covers the formatting edge case
// where the child cleanly exited (cmd.Wait returned nil) before the
// control socket came up. The wrapped error must be a bare
// ErrProcessExited — not an "ErrProcessExited: %!w(<nil>)" string.
func TestWrapProcessExited_NilExitError(t *testing.T) {
	t.Parallel()
	if got := wrapProcessExited(nil); got.Error() != ErrProcessExited.Error() {
		t.Fatalf("wrapProcessExited(nil).Error() = %q; want %q", got.Error(), ErrProcessExited.Error())
	}
	if got := wrapProcessExited(errors.New("boom")); !errors.Is(got, ErrProcessExited) {
		t.Fatalf("wrapProcessExited(err) does not unwrap to ErrProcessExited: %v", got)
	}
}

// TestClientHandle_WaitReady_ProcessExits mirrors the terminal test:
// /bin/true never creates the control socket, so WaitReady must bail
// out with ErrProcessExited rather than spin forever.
func TestClientHandle_WaitReady_ProcessExits(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	doc := validAttachDoc(tmp, filepath.Join(tmp, "c.sock"))

	h, err := NewClient(context.Background(), doc, ClientOptions{
		BinaryPath:        "/bin/true",
		ReadyPollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = h.WaitClose(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = h.WaitReady(ctx)
	if !errors.Is(err, ErrProcessExited) {
		t.Fatalf("WaitReady err = %v; want ErrProcessExited", err)
	}
}

// TestClientHandle_PIDAndSocketPath covers the trivial accessors.
func TestClientHandle_PIDAndSocketPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	sockPath := filepath.Join(tmp, "c.sock")
	doc := validAttachDoc(tmp, sockPath)

	h, err := NewClient(context.Background(), doc, ClientOptions{
		BinaryPath: "/bin/true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if pid := h.PID(); pid <= 0 {
		t.Fatalf("PID = %d; want > 0", pid)
	}
	if got := h.SocketPath(); got != sockPath {
		t.Fatalf("SocketPath = %q; want %q", got, sockPath)
	}
	if got := h.Doc(); got != doc {
		t.Fatalf("Doc = %p; want %p", got, doc)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.WaitClose(ctx)
}
