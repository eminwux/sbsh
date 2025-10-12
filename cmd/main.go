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

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/cmd/sb"
	"github.com/eminwux/sbsh/cmd/sbsh"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{}

	root.AddCommand(sb.NewSbRootCmd())
	root.AddCommand(sbsh.NewSbshRootCmd())

	// Default to info, can be changed at runtime
	var levelVar slog.LevelVar
	levelVar.Set(slog.LevelInfo)

	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: &levelVar})
	handler := &ReformatHandler{inner: textHandler}
	logger := slog.New(handler)

	// Store both logger and levelVar in context using struct keys
	//nolint:revive,staticcheck // ignore revive warning about context keys
	ctx := context.WithValue(context.Background(), "logger", logger)
	//nolint:revive,staticcheck // ignore revive warning about context keys
	ctx = context.WithValue(ctx, "logLevelVar", &levelVar)

	// Select which subtree to run based on the executable name
	exe := filepath.Base(os.Args[0])
	logger.Debug("cmd", "exe", exe, "args", os.Args)
	// Decide which subtree to run by prepending the name as the first arg
	switch exe {
	case "sb":
		logger.Debug("cmd", "sb args", append([]string{"sb"}, os.Args[1:]...))
		root.SetArgs(append([]string{"sb"}, os.Args[1:]...))
	case "sbsh":
		logger.Debug("cmd", "sbsh args", append([]string{"sbsh"}, os.Args[1:]...))
		root.SetArgs(append([]string{"sbsh"}, os.Args[1:]...))
	default:
		logger.Debug("cmd", "default args", append([]string{"sbsh"}, os.Args[1:]...))
		root.SetArgs(append([]string{"sbsh"}, os.Args[1:]...)) // default
	}

	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
