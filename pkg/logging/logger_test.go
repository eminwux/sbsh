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

package logging_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/pkg/logging"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"err":     slog.LevelError,
		"":        slog.LevelInfo,
		"bogus":   slog.LevelInfo,
	}
	for in, want := range cases {
		if got := logging.ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewFileLogger_RejectsEmptyArgs(t *testing.T) {
	if _, err := logging.NewFileLogger("", "info"); err == nil {
		t.Fatal("NewFileLogger(empty path) returned no error")
	}
	if _, err := logging.NewFileLogger(filepath.Join(t.TempDir(), "log"), ""); err == nil {
		t.Fatal("NewFileLogger(empty level) returned no error")
	}
}

func TestNewFileLogger_CreatesParentDirAndFileAtExpectedMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "subdir")
	logPath := filepath.Join(dir, "terminal.log")

	fl, err := logging.NewFileLogger(logPath, "info")
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer fl.File.Close()

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != logging.LogDirMode.Perm() {
		t.Errorf("parent dir mode = %#o, want %#o", got, logging.LogDirMode.Perm())
	}

	fileInfo, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != logging.LogFileMode.Perm() {
		t.Errorf("log file mode = %#o, want %#o", got, logging.LogFileMode.Perm())
	}
}

func TestNewFileLogger_WritesRecordsViaReformatHandler(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "terminal.log")

	fl, err := logging.NewFileLogger(logPath, "debug")
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	fl.Logger.Info("hello", "k", "v")
	if closeErr := fl.File.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "INFO") || !strings.Contains(got, `"hello"`) || !strings.Contains(got, "k=v") {
		t.Errorf("log contents = %q, missing ReformatHandler markers", got)
	}
}

func TestNewFileLogger_AppendsToExistingFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "terminal.log")
	if err := os.WriteFile(logPath, []byte("pre-existing\n"), logging.LogFileMode); err != nil {
		t.Fatalf("seed log file: %v", err)
	}

	fl, err := logging.NewFileLogger(logPath, "info")
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	fl.Logger.Info("after-open")
	if closeErr := fl.File.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(body)
	if !strings.HasPrefix(got, "pre-existing\n") {
		t.Errorf("log truncated existing content: %q", got)
	}
	if !strings.Contains(got, `"after-open"`) {
		t.Errorf("log missing new record: %q", got)
	}
}

func TestNewFileLogger_LevelVarFilters(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "terminal.log")

	fl, err := logging.NewFileLogger(logPath, "warn")
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	fl.Logger.Info("below-threshold")
	fl.Logger.Warn("above-threshold")
	if closeErr := fl.File.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(body)
	if strings.Contains(got, "below-threshold") {
		t.Errorf("expected info record to be filtered, got: %q", got)
	}
	if !strings.Contains(got, "above-threshold") {
		t.Errorf("expected warn record to pass, got: %q", got)
	}
}
