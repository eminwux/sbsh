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

// These tests run under the default build (no integration tag) so the
// command-layer logic in cmd/sb/get is exercised by the standard
// `go test ./...` coverage run, not only the integration-tagged suite.

package get

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func covLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// covCmd returns a cobra command carrying a logger context and a "run-path"
// flag set to runPath, which the Complete*/get* paths resolve through
// config.GetRunPathFromEnvAndFlags.
func covCmd(t *testing.T, runPath string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, covLogger()))
	cmd.Flags().String("run-path", runPath, "")
	return cmd
}

func covWriteTerminal(t *testing.T, runPath, id, name string, state api.TerminalStatusMode) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Metadata:   api.TerminalMetadata{Name: name},
		Spec:       api.TerminalSpec{ID: api.ID(id), Name: name, RunPath: runPath},
		Status:     api.TerminalStatus{State: state, Pid: os.Getpid()},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); writeErr != nil {
		t.Fatalf("write metadata: %v", writeErr)
	}
}

func covWriteClient(t *testing.T, runPath, id, name string, state api.ClientStatusMode) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.ClientsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	doc := api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Name: name},
		Spec:       api.ClientSpec{ID: api.ID(id)},
		Status:     api.ClientStatus{State: state, Pid: os.Getpid()},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); writeErr != nil {
		t.Fatalf("write metadata: %v", writeErr)
	}
}

func covWriteProfiles(t *testing.T, dir string) {
	t.Helper()
	y := `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
`
	if err := os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(y), 0o644); err != nil {
		t.Fatalf("write profiles: %v", err)
	}
}

func Test_NewGetCmd_Cov(t *testing.T) {
	cmd := NewGetCmd()
	if cmd == nil {
		t.Fatal("NewGetCmd returned nil")
	}
	if cmd.Use != "get" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Errorf("get RunE (help): %v", err)
	}
	want := map[string]bool{"terminal": false, "clients": false, "profiles": false}
	for _, sub := range cmd.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("get missing subcommand %q", name)
		}
	}
}

func Test_Terminals_Cov(t *testing.T) {
	runPath := t.TempDir()
	covWriteTerminal(t, runPath, "term1", "alpha", api.Ready)
	covWriteTerminal(t, runPath, "term2", "beta", api.Ready)
	cmd := covCmd(t, runPath)

	viper.Reset()
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, runPath)
	defer viper.Reset()

	if err := listTerminals(cmd, nil); err != nil {
		t.Errorf("listTerminals: %v", err)
	}

	viper.Set(outputFormat, "json")
	if err := getTerminal(cmd, []string{"alpha"}); err != nil {
		t.Errorf("getTerminal: %v", err)
	}
	viper.Set(outputFormat, "")

	names, err := fetchTerminalNames(cmd.Context(), runPath, "al")
	if err != nil {
		t.Fatalf("fetchTerminalNames: %v", err)
	}
	if len(names) != 1 || names[0] != "alpha" {
		t.Errorf("fetchTerminalNames = %v, want [alpha]", names)
	}

	got, _ := CompleteTerminals(cmd, nil, "")
	if len(got) != 2 {
		t.Errorf("CompleteTerminals = %v, want 2 names", got)
	}
	if nilGot, _ := CompleteTerminals(cmd, []string{"already"}, ""); nilGot != nil {
		t.Errorf("CompleteTerminals with positional arg = %v, want nil", nilGot)
	}

	id, err := ResolveTerminalNameToID(cmd.Context(), covLogger(), runPath, "beta")
	if err != nil {
		t.Fatalf("ResolveTerminalNameToID: %v", err)
	}
	if id != "term2" {
		t.Errorf("ResolveTerminalNameToID = %q, want term2", id)
	}
	if _, errResolve := ResolveTerminalNameToID(cmd.Context(), covLogger(), runPath, "ghost"); !errors.Is(
		errResolve,
		errdefs.ErrTerminalNotFound,
	) {
		t.Errorf("expected ErrTerminalNotFound, got: %v", errResolve)
	}
}

func Test_Clients_Cov(t *testing.T) {
	runPath := t.TempDir()
	covWriteClient(t, runPath, "cl1", "client-a", api.ClientReady)
	covWriteClient(t, runPath, "cl2", "client-b", api.ClientReady)
	cmd := covCmd(t, runPath)

	viper.Reset()
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, runPath)
	defer viper.Reset()

	if err := listClients(cmd, nil); err != nil {
		t.Errorf("listClients: %v", err)
	}

	viper.Set(config.SB_GET_CLIENTS_OUTPUT.ViperKey, "json")
	if err := getClient(cmd, []string{"client-a"}); err != nil {
		t.Errorf("getClient: %v", err)
	}
	viper.Set(config.SB_GET_CLIENTS_OUTPUT.ViperKey, "")

	names, err := fetchClientNames(cmd.Context(), runPath, "client-b")
	if err != nil {
		t.Fatalf("fetchClientNames: %v", err)
	}
	if len(names) != 1 || names[0] != "client-b" {
		t.Errorf("fetchClientNames = %v, want [client-b]", names)
	}

	got, _ := CompleteClients(cmd, nil, "")
	if len(got) != 2 {
		t.Errorf("CompleteClients = %v, want 2 names", got)
	}
	if nilGot, _ := CompleteClients(cmd, []string{"already"}, ""); nilGot != nil {
		t.Errorf("CompleteClients with positional arg = %v, want nil", nilGot)
	}

	id, err := ResolveClientNameToID(cmd.Context(), covLogger(), runPath, "client-b")
	if err != nil {
		t.Fatalf("ResolveClientNameToID: %v", err)
	}
	if id != "cl2" {
		t.Errorf("ResolveClientNameToID = %q, want cl2", id)
	}
	if _, errResolve := ResolveClientNameToID(cmd.Context(), covLogger(), runPath, "ghost"); !errors.Is(
		errResolve,
		errdefs.ErrClientNotFound,
	) {
		t.Errorf("expected ErrClientNotFound, got: %v", errResolve)
	}
}

func Test_Profiles_Cov(t *testing.T) {
	dir := t.TempDir()
	covWriteProfiles(t, dir)
	cmd := covCmd(t, dir)

	viper.Reset()
	viper.Set(config.SB_GET_PROFILES_DIR.ViperKey, dir)
	defer viper.Reset()

	if err := listProfiles(cmd, nil); err != nil {
		t.Errorf("listProfiles: %v", err)
	}

	viper.Set(config.SB_GET_PROFILES_OUTPUT.ViperKey, "json")
	if err := getProfile(cmd, []string{"default"}); err != nil {
		t.Errorf("getProfile: %v", err)
	}
	viper.Set(config.SB_GET_PROFILES_OUTPUT.ViperKey, "")

	names, err := fetchProfileNames(cmd.Context(), dir, "def")
	if err != nil {
		t.Fatalf("fetchProfileNames: %v", err)
	}
	if len(names) != 1 || names[0] != "default" {
		t.Errorf("fetchProfileNames = %v, want [default]", names)
	}

	got, _ := completeProfiles(cmd, nil, "")
	if len(got) != 1 {
		t.Errorf("completeProfiles = %v, want 1 name", got)
	}
	if nilGot, _ := completeProfiles(cmd, []string{"already"}, ""); nilGot != nil {
		t.Errorf("completeProfiles with positional arg = %v, want nil", nilGot)
	}
}
