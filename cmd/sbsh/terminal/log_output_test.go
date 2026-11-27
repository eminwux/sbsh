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

package terminal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const stdoutProfileYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: stdoutprof
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
  logOutput: stdout
`

// captureStdout swaps os.Stdout for a pipe until the returned func is
// called, which restores it and returns everything written. Tests using it
// must not run in parallel.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w //nolint:reassign // capture the stdout log sink
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	restored := false
	restore := func() string {
		if restored {
			return ""
		}
		restored = true
		os.Stdout = orig //nolint:reassign // restore the captured stdout
		_ = w.Close()
		return <-done
	}
	t.Cleanup(func() { restore() })
	return restore
}

// setupLogOutputTest resets viper, scrubs ambient SBSH_* env and points the
// run path and profiles dir at temp dirs (the latter holding a stdout
// profile). It returns the run path.
func setupLogOutputTest(t *testing.T) string {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	scrubProfileEnv(t)

	runPath := t.TempDir()
	profilesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(profilesDir, "p.yaml"), []byte(stdoutProfileYAML), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, runPath)
	viper.Set(config.SBSH_ROOT_PROFILES_DIR.ViperKey, profilesDir)
	return runPath
}

func ctxLogger(t *testing.T, cmd *cobra.Command) *slog.Logger {
	t.Helper()
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		t.Fatal("expected a logger in the command context")
	}
	return logger
}

func assertNoTerminalLogFile(t *testing.T, runPath string, id api.ID) {
	t.Helper()
	p := filepath.Join(runPath, defaults.TerminalsRunPath, string(id), "log")
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no log file at %s in stdout mode; stat err = %v", p, err)
	}
}

func Test_processSpec_LogOutputStdout(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T)
	}{
		{name: "flag", prepare: func(_ *testing.T) {
			viper.Set(config.SBSH_TERM_LOG_OUTPUT.ViperKey, "stdout")
		}},
		{name: "env", prepare: func(t *testing.T) {
			t.Setenv(config.SBSH_TERM_LOG_OUTPUT.EnvVar(), "stdout")
		}},
		{name: "profile without flag", prepare: func(_ *testing.T) {
			viper.Set(config.SBSH_TERM_PROFILE.ViperKey, "stdoutprof")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runPath := setupLogOutputTest(t)
			tc.prepare(t)
			cmd := NewTerminalCmd() // binds the env var
			cmd.SetContext(context.Background())

			restore := captureStdout(t)
			var spec *api.TerminalSpec
			if err := processSpec(cmd, &spec, []string{"/bin/echo"}); err != nil {
				restore()
				t.Fatalf("processSpec() error = %v", err)
			}
			ctxLogger(t, cmd).Info("terminal log line to stdout")
			out := restore()

			if spec.LogOutput != api.LogOutputStdout {
				t.Fatalf("LogOutput = %q; want stdout", spec.LogOutput)
			}
			if spec.LogFile != "" {
				t.Fatalf("LogFile = %q; want empty in stdout mode", spec.LogFile)
			}
			if !strings.Contains(out, `INFO "terminal log line to stdout"`) {
				t.Fatalf("stdout = %q; want the terminal log line", out)
			}
			assertNoTerminalLogFile(t, runPath, spec.ID)
		})
	}
}

func Test_processSpec_LogOutputFlagOverridesProfile(t *testing.T) {
	runPath := setupLogOutputTest(t)
	viper.Set(config.SBSH_TERM_PROFILE.ViperKey, "stdoutprof")
	viper.Set(config.SBSH_TERM_LOG_OUTPUT.ViperKey, "file")
	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	var spec *api.TerminalSpec
	if err := processSpec(cmd, &spec, []string{"/bin/echo"}); err != nil {
		t.Fatalf("processSpec() error = %v", err)
	}
	want := filepath.Join(runPath, defaults.TerminalsRunPath, string(spec.ID), "log")
	if spec.LogOutput != api.LogOutputFile || spec.LogFile != want {
		t.Fatalf("LogOutput=%q LogFile=%q; want file at %q", spec.LogOutput, spec.LogFile, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected log file at %s: %v", want, err)
	}
}

func Test_processSpec_LogOutputDefaultUnchanged(t *testing.T) {
	runPath := setupLogOutputTest(t)
	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	var spec *api.TerminalSpec
	if err := processSpec(cmd, &spec, []string{"/bin/echo"}); err != nil {
		t.Fatalf("processSpec() error = %v", err)
	}
	want := filepath.Join(runPath, defaults.TerminalsRunPath, string(spec.ID), "log")
	if spec.LogOutput != api.LogOutputFile {
		t.Fatalf("LogOutput = %q; want file", spec.LogOutput)
	}
	if spec.LogFile != want {
		t.Fatalf("LogFile = %q; want default %q", spec.LogFile, want)
	}
	ctxLogger(t, cmd).Info("to the file")
	b, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(b), `"to the file"`) {
		t.Fatalf("log file = %q; want the logged line", b)
	}
}

func Test_processSpec_LogOutputStdoutWithLogFileConflicts(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func()
	}{
		{name: "flag", set: func() { viper.Set(config.SBSH_TERM_LOG_OUTPUT.ViperKey, "stdout") }},
		{name: "profile", set: func() { viper.Set(config.SBSH_TERM_PROFILE.ViperKey, "stdoutprof") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupLogOutputTest(t)
			tc.set()
			explicit := filepath.Join(t.TempDir(), "explicit.log")
			viper.Set(config.SBSH_TERM_LOG_FILE.ViperKey, explicit)
			cmd := NewTerminalCmd()
			cmd.SetContext(context.Background())

			captureStdout(t) // keep fallback log lines off the test output
			var spec *api.TerminalSpec
			err := processSpec(cmd, &spec, []string{"/bin/echo"})
			if !errors.Is(err, errdefs.ErrConflictingFlags) {
				t.Fatalf("err = %v; want ErrConflictingFlags", err)
			}
		})
	}
}

func Test_processSpec_LogOutputInvalidFlag(t *testing.T) {
	setupLogOutputTest(t)
	viper.Set(config.SBSH_TERM_LOG_OUTPUT.ViperKey, "stderr")
	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	var spec *api.TerminalSpec
	err := processSpec(cmd, &spec, []string{"/bin/echo"})
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("err = %v; want ErrInvalidFlag", err)
	}
}

func Test_processSpec_SuppliedSpecLogOutput(t *testing.T) {
	t.Run("stdout without log file", func(t *testing.T) {
		cmd := NewTerminalCmd()
		cmd.SetContext(context.Background())
		spec := &api.TerminalSpec{LogLevel: "info", LogOutput: api.LogOutputStdout}

		restore := captureStdout(t)
		if err := processSpec(cmd, &spec, nil); err != nil {
			restore()
			t.Fatalf("processSpec() error = %v", err)
		}
		ctxLogger(t, cmd).Info("from supplied spec")
		if out := restore(); !strings.Contains(out, `"from supplied spec"`) {
			t.Fatalf("stdout = %q; want the log line", out)
		}
	})
	t.Run("stdout with log file rejected", func(t *testing.T) {
		cmd := NewTerminalCmd()
		cmd.SetContext(context.Background())
		spec := &api.TerminalSpec{
			LogLevel:  "info",
			LogOutput: api.LogOutputStdout,
			LogFile:   filepath.Join(t.TempDir(), "log"),
		}
		if err := processSpec(cmd, &spec, nil); !errors.Is(err, errdefs.ErrInvalidJSONSpec) {
			t.Fatalf("err = %v; want ErrInvalidJSONSpec", err)
		}
	})
	t.Run("empty normalizes to file", func(t *testing.T) {
		cmd := NewTerminalCmd()
		cmd.SetContext(context.Background())
		spec := &api.TerminalSpec{LogLevel: "info", LogFile: filepath.Join(t.TempDir(), "log")}
		if err := processSpec(cmd, &spec, nil); err != nil {
			t.Fatalf("processSpec() error = %v", err)
		}
		if spec.LogOutput != api.LogOutputFile {
			t.Fatalf("LogOutput = %q; want file", spec.LogOutput)
		}
	})
}

func Test_setLoggingVarsFromFlags_StdoutSkipsDefaultLogFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, t.TempDir())

	if setLoggingVarsFromFlags(api.LogOutputStdout) {
		t.Fatal("expected no defaulted log file in stdout mode")
	}
	if got := viper.GetString(config.SBSH_TERM_LOG_FILE.ViperKey); got != "" {
		t.Fatalf("log file = %q; want empty in stdout mode", got)
	}
}
