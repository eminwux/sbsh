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

package prune

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/spf13/viper"
)

func pruneTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func pruneTestCtx() context.Context {
	return context.WithValue(context.Background(), types.CtxLogger, pruneTestLogger())
}

func Test_NewPruneCmd_Metadata(t *testing.T) {
	cmd := NewPruneCmd()
	if cmd == nil {
		t.Fatal("NewPruneCmd returned nil")
	}
	if cmd.Use != "prune" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Errorf("prune RunE (help) returned error: %v", err)
	}
	var sawTerminals, sawClients bool
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "terminals":
			sawTerminals = true
		case "clients":
			sawClients = true
		}
	}
	if !sawTerminals || !sawClients {
		t.Errorf("prune missing subcommands: terminals=%v clients=%v", sawTerminals, sawClients)
	}
}

func Test_PruneTerminals_RunE_EmptyRunPath(t *testing.T) {
	viper.Reset()
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, t.TempDir())
	cmd := NewPruneTerminalsCmd()
	cmd.SetContext(pruneTestCtx())

	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("prune terminals RunE: %v", err)
	}
}

func Test_PruneClients_RunE_EmptyRunPath(t *testing.T) {
	viper.Reset()
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, t.TempDir())
	cmd := NewPruneClientsCmd()
	cmd.SetContext(pruneTestCtx())

	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("prune clients RunE: %v", err)
	}
}
