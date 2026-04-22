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

package builder_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/builder"
)

func TestBuildClientDoc_EmptyRunPath(t *testing.T) {
	_, err := builder.BuildClientDoc(context.Background(), testLogger(), "")
	if !errors.Is(err, errdefs.ErrRunPathRequired) {
		t.Fatalf("expected ErrRunPathRequired, got %v", err)
	}
}

func TestBuildClientDoc_Defaults(t *testing.T) {
	runPath := t.TempDir()
	doc, err := builder.BuildClientDoc(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.APIVersion != api.APIVersionV1Beta1 {
		t.Fatalf("apiVersion: want %q, got %q", api.APIVersionV1Beta1, doc.APIVersion)
	}
	if doc.Kind != api.KindClient {
		t.Fatalf("kind: want %q, got %q", api.KindClient, doc.Kind)
	}
	if doc.Metadata.Name == "" {
		t.Fatal("expected a default name, got empty")
	}
	if string(doc.Spec.ID) == "" {
		t.Fatal("expected a default ID, got empty")
	}
	if doc.Spec.RunPath != runPath {
		t.Fatalf("runPath: want %q, got %q", runPath, doc.Spec.RunPath)
	}
	wantSocket := filepath.Join(runPath, defaults.ClientsRunPath, string(doc.Spec.ID), "socket")
	if doc.Spec.SockerCtrl != wantSocket {
		t.Fatalf("socket: want %q, got %q", wantSocket, doc.Spec.SockerCtrl)
	}
	if !doc.Spec.DetachKeystroke {
		t.Fatal("expected DetachKeystroke to default true")
	}
	if doc.Spec.ClientMode != api.RunNewTerminal {
		t.Fatalf("mode: want RunNewTerminal, got %v", doc.Spec.ClientMode)
	}
	if doc.Spec.TerminalSpec == nil {
		t.Fatal("expected non-nil TerminalSpec")
	}
}

func TestBuildClientDoc_Overrides(t *testing.T) {
	runPath := t.TempDir()
	embed := &api.TerminalSpec{ID: "embedded"}
	doc, err := builder.BuildClientDoc(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithClientID("client-id"),
		builder.WithClientName("client-name"),
		builder.WithClientSocketFile("/tmp/custom.sock"),
		builder.WithClientLogFile("/tmp/custom.log"),
		builder.WithClientMode(api.AttachToTerminal),
		builder.WithClientDetachKeystroke(false),
		builder.WithClientTerminalSpec(embed),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(doc.Spec.ID) != "client-id" {
		t.Fatalf("id: want client-id, got %q", doc.Spec.ID)
	}
	if doc.Metadata.Name != "client-name" {
		t.Fatalf("name: want client-name, got %q", doc.Metadata.Name)
	}
	if doc.Spec.SockerCtrl != "/tmp/custom.sock" {
		t.Fatalf("socket: got %q", doc.Spec.SockerCtrl)
	}
	if doc.Spec.LogFile != "/tmp/custom.log" {
		t.Fatalf("log: got %q", doc.Spec.LogFile)
	}
	if doc.Spec.ClientMode != api.AttachToTerminal {
		t.Fatalf("mode: want AttachToTerminal, got %v", doc.Spec.ClientMode)
	}
	if doc.Spec.DetachKeystroke {
		t.Fatal("expected DetachKeystroke=false after WithClientDetachKeystroke(false)")
	}
	if doc.Spec.TerminalSpec != embed {
		t.Fatal("expected embedded TerminalSpec to be the same pointer")
	}
}

func TestBuildClientDoc_OptionOverrideOrder(t *testing.T) {
	runPath := t.TempDir()
	doc, err := builder.BuildClientDoc(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithClientID("first"),
		builder.WithClientID("second"),
		builder.WithClientName("a"),
		builder.WithClientName("b"),
		builder.WithClientMode(api.AttachToTerminal),
		builder.WithClientMode(api.RunNewTerminal),
		builder.WithClientDetachKeystroke(false),
		builder.WithClientDetachKeystroke(true),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(doc.Spec.ID) != "second" {
		t.Fatalf("id: want second, got %q", doc.Spec.ID)
	}
	if doc.Metadata.Name != "b" {
		t.Fatalf("name: want b, got %q", doc.Metadata.Name)
	}
	if doc.Spec.ClientMode != api.RunNewTerminal {
		t.Fatalf("mode: want RunNewTerminal (last-wins), got %v", doc.Spec.ClientMode)
	}
	if !doc.Spec.DetachKeystroke {
		t.Fatal("expected DetachKeystroke=true (last-wins)")
	}
}

func TestBuildClientDoc_NilOptionSafe(t *testing.T) {
	runPath := t.TempDir()
	_, err := builder.BuildClientDoc(
		context.Background(),
		testLogger(),
		runPath,
		nil,
		builder.WithClientID("x"),
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Nil TerminalSpec passed explicitly still normalizes to an empty
// non-nil spec so controller code never dereferences nil.
func TestBuildClientDoc_NilTerminalSpec(t *testing.T) {
	runPath := t.TempDir()
	doc, err := builder.BuildClientDoc(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithClientTerminalSpec(nil),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Spec.TerminalSpec == nil {
		t.Fatal("expected non-nil TerminalSpec after WithClientTerminalSpec(nil)")
	}
}

// Default socket path embeds the resolved (possibly random) ID so
// that collisions across concurrent clients are avoided.
func TestBuildClientDoc_SocketContainsResolvedID(t *testing.T) {
	runPath := t.TempDir()
	doc, err := builder.BuildClientDoc(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithClientID("explicit-id"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(doc.Spec.SockerCtrl, "explicit-id") {
		t.Fatalf("expected socket path to contain ID, got %q", doc.Spec.SockerCtrl)
	}
}
