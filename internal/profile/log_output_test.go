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

package profile

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func logOutputProfileYAML(name, logOutput string) string {
	y := "apiVersion: sbsh/v1beta1\nkind: TerminalProfile\nmetadata:\n  name: " + name +
		"\nspec:\n  runTarget: local\n  shell:\n    cmd: /bin/sh\n"
	if logOutput != "" {
		y += "  logOutput: " + logOutput + "\n"
	}
	return y
}

func writeLogOutputProfilesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, lo := range map[string]string{"plain": "", "fileprof": "file", "stdoutprof": "stdout"} {
		if err := os.WriteFile(
			filepath.Join(dir, name+".yaml"), []byte(logOutputProfileYAML(name, lo)), 0o644,
		); err != nil {
			t.Fatalf("write profile: %v", err)
		}
	}
	return dir
}

// warnCounter is a slog.Handler that counts WARN-or-above records.
type warnCounter struct {
	mu    sync.Mutex
	warns int
}

func (h *warnCounter) Enabled(context.Context, slog.Level) bool { return true }
func (h *warnCounter) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		h.mu.Lock()
		h.warns++
		h.mu.Unlock()
	}
	return nil
}
func (h *warnCounter) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *warnCounter) WithGroup(string) slog.Handler      { return h }

//nolint:gocognit // table of precedence scenarios asserted inline
func TestBuildTerminalSpecFromProfile_LogOutputPrecedence(t *testing.T) {
	profilesDir := writeLogOutputProfilesDir(t)

	tests := []struct {
		name             string
		profile          string
		override         string
		allowStdout      bool
		logFile          string
		logFileDefaulted bool
		wantOutput       string
		wantDefaultLog   bool // spec.LogFile == runPath/terminals/<id>/log
		wantErr          error
		wantWarns        int
	}{
		{
			name: "no value anywhere keeps file", profile: "plain", allowStdout: true,
			wantOutput: api.LogOutputFile, wantDefaultLog: true,
		},
		{
			name: "profile file", profile: "fileprof", allowStdout: true,
			wantOutput: api.LogOutputFile, wantDefaultLog: true,
		},
		{
			name: "profile stdout, no override", profile: "stdoutprof", allowStdout: true,
			wantOutput: api.LogOutputStdout,
		},
		{
			name: "override file beats profile stdout", profile: "stdoutprof", override: "file", allowStdout: true,
			wantOutput: api.LogOutputFile, wantDefaultLog: true,
		},
		{
			name: "override stdout beats profile file", profile: "fileprof", override: "stdout", allowStdout: true,
			wantOutput: api.LogOutputStdout,
		},
		{
			name: "override stdout, no profile value", profile: "plain", override: "stdout", allowStdout: true,
			wantOutput: api.LogOutputStdout,
		},
		{
			name: "profile stdout on client path downgrades with one warning", profile: "stdoutprof",
			wantOutput: api.LogOutputFile, wantDefaultLog: true, wantWarns: 1,
		},
		{
			name: "profile stdout drops a caller-defaulted log file", profile: "stdoutprof", allowStdout: true,
			logFile: "/run/x/terminals/t/log", logFileDefaulted: true, wantOutput: api.LogOutputStdout,
		},
		{
			name: "override stdout with explicit log file conflicts", profile: "plain", override: "stdout",
			allowStdout: true, logFile: "/tmp/explicit.log", wantErr: errdefs.ErrConflictingFlags,
		},
		{
			name: "profile stdout with explicit log file conflicts", profile: "stdoutprof",
			allowStdout: true, logFile: "/tmp/explicit.log", wantErr: errdefs.ErrConflictingFlags,
		},
		{
			name: "invalid override", profile: "plain", override: "stderr", allowStdout: true,
			wantErr: errdefs.ErrInvalidFlag,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			counter := &warnCounter{}
			runPath := t.TempDir()
			input := &BuildTerminalSpecParams{
				RunPath:          runPath,
				ProfilesDir:      profilesDir,
				ProfileName:      tc.profile,
				LogFile:          tc.logFile,
				LogFileDefaulted: tc.logFileDefaulted,
				LogOutput:        tc.override,
				AllowStdoutLog:   tc.allowStdout,
			}
			spec, err := BuildTerminalSpecFromProfile(context.Background(), slog.New(counter), input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v; want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if spec.LogOutput != tc.wantOutput {
				t.Errorf("LogOutput = %q; want %q", spec.LogOutput, tc.wantOutput)
			}
			if tc.wantOutput == api.LogOutputStdout {
				if spec.LogFile != "" || input.LogFile != "" {
					t.Errorf("stdout mode: spec.LogFile=%q input.LogFile=%q; want both empty",
						spec.LogFile, input.LogFile)
				}
			}
			if tc.wantDefaultLog {
				want := filepath.Join(runPath, defaults.TerminalsRunPath, string(spec.ID), "log")
				if spec.LogFile != want {
					t.Errorf("LogFile = %q; want default %q", spec.LogFile, want)
				}
			}
			if counter.warns != tc.wantWarns {
				t.Errorf("warnings = %d; want %d", counter.warns, tc.wantWarns)
			}
		})
	}
}

func TestCreateTerminalFromProfile_InvalidLogOutputRejected(t *testing.T) {
	doc := &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata:   api.TerminalProfileMetadata{Name: "bad"},
		Spec: api.TerminalProfileSpec{
			Shell:     api.ShellSpec{Cmd: "/bin/sh"},
			LogOutput: "syslog",
		},
	}
	_, err := CreateTerminalFromProfile(doc)
	if err == nil || !strings.Contains(err.Error(), `invalid spec.logOutput in profile "bad"`) {
		t.Fatalf("err = %v; want invalid spec.logOutput error", err)
	}
}

// With a stdout override and caller-supplied capture/socket paths, LogFile
// stays empty, so the "all three artifact paths supplied" MetadataDir
// derivation must not fire: metadata.json keeps the legacy layout.
func TestBuildTerminalSpecInline_StdoutKeepsLegacyMetadataLayout(t *testing.T) {
	dir := t.TempDir()
	input := &BuildTerminalSpecParams{
		RunPath:        t.TempDir(),
		CaptureFile:    filepath.Join(dir, "capture"),
		SocketFile:     filepath.Join(dir, "socket"),
		LogOutput:      api.LogOutputStdout,
		AllowStdoutLog: true,
	}
	spec, err := BuildTerminalSpecInline(context.Background(), testLogger(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogOutput != api.LogOutputStdout || spec.LogFile != "" {
		t.Fatalf("LogOutput=%q LogFile=%q; want stdout and empty", spec.LogOutput, spec.LogFile)
	}
	if spec.MetadataDir != "" {
		t.Fatalf("MetadataDir = %q; want empty (legacy layout)", spec.MetadataDir)
	}
}

// The inline (pkg/builder) lane never sets AllowStdoutLog, so a stdout
// override there is downgraded rather than silently dropping the log file.
func TestBuildTerminalSpecInline_StdoutNotAllowedDowngrades(t *testing.T) {
	spec, err := BuildTerminalSpecInline(context.Background(), testLogger(), &BuildTerminalSpecParams{
		RunPath:   t.TempDir(),
		LogOutput: api.LogOutputStdout,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogOutput != api.LogOutputFile || spec.LogFile == "" {
		t.Fatalf("LogOutput=%q LogFile=%q; want file with a default path", spec.LogOutput, spec.LogFile)
	}
}
