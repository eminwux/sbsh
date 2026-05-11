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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// LogDirMode is the permission mode used when creating the parent
// directory of a per-terminal log file.
const LogDirMode os.FileMode = 0o700

// LogFileMode is the permission mode used when creating a per-terminal
// log file. It matches the mode the terminal runner re-applies via
// chmod during StartTerminal, so pre-creating at this mode avoids a
// no-op re-chmod and the chmod-on-missing-file failure mode.
const LogFileMode os.FileMode = 0o600

// ParseLevel maps an sbsh log-level string ("debug", "info", "warn",
// "error") to a slog.Level. Unknown values resolve to slog.LevelInfo.
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
		return slog.LevelInfo
	}
}

// FileLogger bundles the file-backed slog.Logger and the resources
// that back it. File is the underlying sink; the caller owns it and
// must Close it during shutdown to flush and release the OS handle.
// LevelVar exposes the configured threshold so callers can adjust the
// minimum level at runtime.
type FileLogger struct {
	Logger   *slog.Logger
	LevelVar *slog.LevelVar
	Handler  *ReformatHandler
	File     *os.File
}

// NewFileLogger creates the parent directory of logfile at LogDirMode
// if needed, opens or creates logfile with O_CREATE|O_WRONLY|O_APPEND
// at LogFileMode, and wires a structured slog.Logger that writes to
// it using ReformatHandler.
//
// This is the pre-creation step the sbsh CLI performs before driving
// the terminal runner. Out-of-tree callers using pkg/terminal/server
// with a TerminalSpec that names a LogFile (e.g. one produced by
// pkg/builder.BuildTerminalSpec) must call NewFileLogger (or otherwise
// create the file at LogFileMode) before Serve — the runner re-chmods
// the path during StartTerminal but does not create it.
//
// Both logfile and loglevel are required.
func NewFileLogger(logfile, loglevel string) (*FileLogger, error) {
	if logfile == "" || loglevel == "" {
		return nil, fmt.Errorf("logging: logfile=%q, loglevel=%q must not be empty", logfile, loglevel)
	}
	if err := os.MkdirAll(filepath.Dir(logfile), LogDirMode); err != nil {
		return nil, fmt.Errorf("logging: mkdir %s: %w", filepath.Dir(logfile), err)
	}
	f, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFileMode)
	if err != nil {
		return nil, fmt.Errorf("logging: open %s: %w", logfile, err)
	}

	levelVar := new(slog.LevelVar)
	levelVar.Set(ParseLevel(loglevel))

	handler := &ReformatHandler{
		Inner:  slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar}),
		Writer: f,
	}
	logger := slog.New(handler)

	return &FileLogger{
		Logger:   logger,
		LevelVar: levelVar,
		Handler:  handler,
		File:     f,
	}, nil
}
