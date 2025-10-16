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

package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func ParseLevel(lvl string) slog.Level {
	switch lvl {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default:
		// default if unknown
		return slog.LevelInfo
	}
}

func SetupFileLogger(cmd *cobra.Command, logfile string, loglevel string) error {
	if cmd == nil || logfile == "" || loglevel == "" {
		return errors.New("cmd, logfile, and loglevel must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(logfile), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		return err
	}

	f, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		return err
	}

	// Create a new logger that writes to the file with the specified log level
	levelVar := new(slog.LevelVar)
	levelVar.Set(ParseLevel(loglevel))

	handler := &ReformatHandler{
		Inner:  slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar}),
		Writer: f,
	}
	logger := slog.New(handler)

	// Store both logger and levelVar in context using struct keys
	ctx := cmd.Context()
	ctx = context.WithValue(ctx, CtxLogger, logger)
	ctx = context.WithValue(ctx, CtxLevelVar, &levelVar)
	ctx = context.WithValue(ctx, CtxHandler, handler)

	cmd.SetContext(ctx)
	return nil
}
