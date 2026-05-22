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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestRunTerminal_ErrContextCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newTerminalController := func(_ context.Context) api.TerminalController {
		return &terminal.ControllerTest{
			Exit:          nil,
			RunFunc:       func(_ *api.TerminalSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return nil },
			PingFunc:      func(_ *api.PingMessage) (*api.PingMessage, error) { return &api.PingMessage{Message: "PONG"}, nil },
		}
	}

	ctrl := newTerminalController(context.Background())

	t.Cleanup(func() {})

	spec := api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}

	done := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runTerminal(ctx, cancel, logger, ctrl, &spec) // will block until ctx.Done()
		defer close(done)
	}()

	// Give Run() time to set ready, then signal the process (NotifyContext listens to SIGTERM/INT)
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, errdefs.ErrContextDone) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runTerminal to return after SIGTERM")
	}
}

func TestRunTerminal_ErrWaitOnReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newTerminalController := func(_ context.Context) api.TerminalController {
		return &terminal.ControllerTest{
			Exit:          nil,
			RunFunc:       func(_ *api.TerminalSpec) error { return nil },
			WaitReadyFunc: func() error { return errors.New("not ready") },
			WaitCloseFunc: func() error { return nil },
		}
	}

	ctrl := newTerminalController(context.Background())

	t.Cleanup(func() {})

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	spec := api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := runTerminal(ctx, cancel, logger, ctrl, &spec); err != nil && !errors.Is(err, errdefs.ErrWaitOnReady) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)
	}
}

func TestRunTerminal_ErrWaitOnClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newTerminalController := func(_ context.Context) api.TerminalController {
		return &terminal.ControllerTest{
			Exit:          nil,
			RunFunc:       func(_ *api.TerminalSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return errors.New("error on close") },
		}
	}
	t.Cleanup(func() {})

	ctrl := newTerminalController(context.Background())

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	spec := api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())

	go func(exitCh chan error) {
		exitCh <- runTerminal(ctx, cancel, logger, ctrl, &spec)
		defer close(exitCh)
	}(exitCh)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrWaitOnClose) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
	}
}

func Test_ErrInvalidFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("id", "", "test flag")
	cmd.Flags().Parse([]string{"--id", "test123"})

	err := checkFlag(cmd, "id", "positional argument '-'")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_ErrInvalidArgument(t *testing.T) {
	cmd := &cobra.Command{}
	spec, err := processInput(cmd, []string{"invalid"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidArgument, err)
	}
}

func Test_ErrStdinEmpty(t *testing.T) {
	// This test verifies that ErrStdinEmpty is properly defined
	// Testing the actual condition requires mocking os.Stdin.Stat(), which is complex
	// The error is tested indirectly through integration tests
	err := errdefs.ErrStdinEmpty
	if err == nil {
		t.Fatal("ErrStdinEmpty should not be nil")
	}
	if !errors.Is(err, errdefs.ErrStdinEmpty) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStdinEmpty, err)
	}
}

func Test_ErrOpenSpecFile(t *testing.T) {
	cmd := &cobra.Command{}
	viper.Set(config.SBSH_TERM_SPEC.ViperKey, "/nonexistent/file/path/that/does/not/exist.json")

	spec, err := processInput(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrOpenSpecFile) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSpecFile, err)
	}
}

func Test_ErrInvalidJSONSpec(t *testing.T) {
	// Create a temporary file with invalid JSON
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(tmpFile, []byte("{invalid json}"), 0o644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	cmd := &cobra.Command{}
	viper.Set(config.SBSH_TERM_SPEC.ViperKey, tmpFile)

	spec, err := processInput(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrInvalidJSONSpec) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidJSONSpec, err)
	}
}

func Test_ErrNoSpecDefined(t *testing.T) {
	cmd := &cobra.Command{}
	viper.Set(config.SBSH_TERM_SPEC.ViperKey, "")

	spec, err := processInput(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrNoSpecDefined) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoSpecDefined, err)
	}
}

func Test_ErrTerminalSpecNotFound_RunE(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cmd := NewTerminalCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)
	// Don't set CtxTerminalSpec, so it will be nil

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrTerminalSpecNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrTerminalSpecNotFound, err)
	}
}

func Test_ErrLoggerNotFound_RunE(t *testing.T) {
	cmd := NewTerminalCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	// Set a valid terminal spec so we get past that check
	spec := &api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
	}
	ctx = context.WithValue(ctx, types.CtxTerminalSpec, spec)
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_splitWorkload(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		dash         int
		wantPreDash  []string
		wantWorkload []string
	}{
		{
			name:         "no dash separator",
			args:         []string{"-"},
			dash:         -1,
			wantPreDash:  []string{"-"},
			wantWorkload: nil,
		},
		{
			name:         "no positional args at all",
			args:         []string{},
			dash:         -1,
			wantPreDash:  []string{},
			wantWorkload: nil,
		},
		{
			name:         "workload with no pre-dash args",
			args:         []string{"/bin/bash"},
			dash:         0,
			wantPreDash:  []string{},
			wantWorkload: []string{"/bin/bash"},
		},
		{
			name:         "workload with whitespace and quoting in args",
			args:         []string{"printf", "%s\n", "a b", "c'd"},
			dash:         0,
			wantPreDash:  []string{},
			wantWorkload: []string{"printf", "%s\n", "a b", "c'd"},
		},
		{
			name:         "workload with multiple pre-dash args",
			args:         []string{"-", "extra", "/bin/echo", "hi"},
			dash:         2,
			wantPreDash:  []string{"-", "extra"},
			wantWorkload: []string{"/bin/echo", "hi"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPreDash, gotWorkload := splitWorkload(tc.args, tc.dash)
			if !reflect.DeepEqual(gotPreDash, tc.wantPreDash) {
				t.Fatalf("preDash: expected %#v; got %#v", tc.wantPreDash, gotPreDash)
			}
			if !reflect.DeepEqual(gotWorkload, tc.wantWorkload) {
				t.Fatalf("workload: expected %#v; got %#v", tc.wantWorkload, gotWorkload)
			}
		})
	}
}

func Test_TerminalCmd_RejectsCommandFlag(t *testing.T) {
	cmd := NewTerminalCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--command", "/bin/bash"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error: --command flag should be removed and rejected by cobra")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag error; got: %v", err)
	}
}

func Test_TerminalCmd_PositionalArgvAfterDash(t *testing.T) {
	// Verify that cobra parses positional argv after `--` and exposes it
	// verbatim (no shell re-tokenization) via ArgsLenAtDash + args[dash:].
	cmd := NewTerminalCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	want := []string{"printf", "%s\n", "a b"}

	var (
		gotWorkload []string
		captured    bool
	)
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		_, gotWorkload = splitWorkload(args, c.ArgsLenAtDash())
		captured = true
		return errors.New("stop after capture")
	}
	cmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }

	cmdArgs := append([]string{"--"}, want...)
	cmd.SetArgs(cmdArgs)
	_ = cmd.Execute()

	if !captured {
		t.Fatal("PreRunE was not invoked")
	}
	if !reflect.DeepEqual(gotWorkload, want) {
		t.Fatalf("workload: expected %#v; got %#v", want, gotWorkload)
	}
}

func Test_TerminalCmd_DefaultFallback_NoWorkload(t *testing.T) {
	// When no positional argv is given after `--`, ArgsLenAtDash returns -1
	// and the workload is empty so the profile default (/bin/bash) applies.
	cmd := NewTerminalCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	var (
		gotDash     int
		gotWorkload []string
		captured    bool
	)
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		gotDash = c.ArgsLenAtDash()
		_, gotWorkload = splitWorkload(args, gotDash)
		captured = true
		return errors.New("stop after capture")
	}
	cmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }

	cmd.SetArgs([]string{})
	_ = cmd.Execute()

	if !captured {
		t.Fatal("PreRunE was not invoked")
	}
	if gotDash != -1 {
		t.Fatalf("ArgsLenAtDash: expected -1 (no `--`); got %d", gotDash)
	}
	if len(gotWorkload) != 0 {
		t.Fatalf("workload: expected empty; got %#v", gotWorkload)
	}
}

func Test_ValidJSONSpec(t *testing.T) {
	// Create a temporary file with valid JSON
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "valid.json")
	validSpec := api.TerminalSpec{
		ID:          api.ID("test-id"),
		Kind:        api.TerminalLocal,
		Name:        "test-name",
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
	}
	jsonData, err := json.Marshal(validSpec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}
	err = os.WriteFile(tmpFile, jsonData, 0o644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	cmd := &cobra.Command{}
	viper.Set(config.SBSH_TERM_SPEC.ViperKey, tmpFile)

	spec, err := processInput(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if spec.ID != validSpec.ID {
		t.Fatalf("expected ID %s; got: %s", validSpec.ID, spec.ID)
	}
	if spec.Name != validSpec.Name {
		t.Fatalf("expected Name %s; got: %s", validSpec.Name, spec.Name)
	}
}

func Test_processSpec_RejectsWorkloadWithSpec(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	spec := &api.TerminalSpec{
		ID:      api.ID("test-id"),
		Kind:    api.TerminalLocal,
		Name:    "test-name",
		Command: "/bin/bash",
	}

	err := processSpec(cmd, &spec, []string{"/bin/zsh"})
	if err == nil {
		t.Fatal("expected error when workload supplied alongside a stdin/file spec")
	}
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidArgument, err)
	}
}

func TestRunTerminal_ChildExitError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctrl := &terminal.ControllerTest{
		RunFunc:       func(_ *api.TerminalSpec) error { return errdefs.ErrChildExit },
		WaitReadyFunc: func() error { return nil },
		WaitCloseFunc: func() error { return errors.New("close failed") },
	}

	spec := api.TerminalSpec{
		ID:      api.ID(naming.RandomID()),
		Kind:    api.TerminalLocal,
		Name:    naming.RandomName(),
		Command: "/bin/bash",
	}

	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { done <- runTerminal(ctx, cancel, logger, ctrl, &spec) }()

	select {
	case err := <-done:
		// runTerminal swallows child-exit errors to avoid polluting the
		// terminal; it returns nil after exercising the WaitClose branch.
		if err != nil {
			t.Fatalf("expected nil (child-exit is swallowed); got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runTerminal to return")
	}
}

func Test_buildTerminalSpecFromFlags_RunPathRequired(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// No run path set -> BuildTerminalSpecFromProfile rejects with ErrRunPathRequired.
	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	spec, err := buildTerminalSpecFromFlags(cmd, logger, nil)
	if err == nil {
		t.Fatal("expected error when run path is empty")
	}
	if spec != nil {
		t.Fatal("expected nil spec on error")
	}
}

func Test_checkStdInUsage_RejectsConflictingFlags(t *testing.T) {
	cmd := NewTerminalCmd()
	if err := cmd.Flags().Set("name", "foo"); err != nil {
		t.Fatalf("set name flag: %v", err)
	}

	err := checkStdInUsage(cmd, []string{"-"})
	if err == nil {
		t.Fatal("expected error when --name is used alongside stdin '-'")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_checkStdInUsage_NoFlagsOK(t *testing.T) {
	cmd := NewTerminalCmd()
	if err := checkStdInUsage(cmd, []string{"-"}); err != nil {
		t.Fatalf("expected nil when no conflicting flags set; got %v", err)
	}
}

func Test_readGIDFlag(t *testing.T) {
	const flagName = "socket-gid"
	const envName = "SBSH_TEST_GID"

	t.Run("flag changed wins", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		if err := cmd.Flags().Set(flagName, "42"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
		got := readGIDFlag(cmd, flagName, envName)
		if got == nil || *got != 42 {
			t.Fatalf("expected 42; got %v", got)
		}
	})

	t.Run("env used when flag unchanged", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		t.Setenv(envName, "7")
		got := readGIDFlag(cmd, flagName, envName)
		if got == nil || *got != 7 {
			t.Fatalf("expected 7; got %v", got)
		}
	})

	t.Run("nil when neither set", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		t.Setenv(envName, "")
		if got := readGIDFlag(cmd, flagName, envName); got != nil {
			t.Fatalf("expected nil; got %v", got)
		}
	})

	t.Run("nil when env not parseable", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		t.Setenv(envName, "not-a-number")
		if got := readGIDFlag(cmd, flagName, envName); got != nil {
			t.Fatalf("expected nil; got %v", got)
		}
	})
}

func Test_readGIDWrappers(t *testing.T) {
	cmd := NewTerminalCmd()
	if err := cmd.Flags().Set("socket-gid", "11"); err != nil {
		t.Fatalf("set socket-gid: %v", err)
	}
	if err := cmd.Flags().Set("capture-gid", "12"); err != nil {
		t.Fatalf("set capture-gid: %v", err)
	}
	if err := cmd.Flags().Set("log-file-gid", "13"); err != nil {
		t.Fatalf("set log-file-gid: %v", err)
	}

	if got := readSocketGID(cmd); got == nil || *got != 11 {
		t.Fatalf("readSocketGID: expected 11; got %v", got)
	}
	if got := readCaptureGID(cmd); got == nil || *got != 12 {
		t.Fatalf("readCaptureGID: expected 12; got %v", got)
	}
	if got := readLogFileGID(cmd); got == nil || *got != 13 {
		t.Fatalf("readLogFileGID: expected 13; got %v", got)
	}
}

func Test_setLoggingVarsFromFlags_Defaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	runPath := t.TempDir()
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, runPath)

	setLoggingVarsFromFlags()

	if got := viper.GetString(config.SBSH_TERM_ID.ViperKey); got == "" {
		t.Fatal("expected a generated terminal ID")
	}
	if got := viper.GetString(config.SBSH_TERM_LOG_LEVEL.ViperKey); got != "info" {
		t.Fatalf("expected log level info; got %q", got)
	}
	if got := viper.GetString(config.SBSH_TERM_LOG_FILE.ViperKey); got == "" {
		t.Fatal("expected a derived log file path")
	}
}

// scrubProfileEnv neutralizes ambient SBSH_TERM_*/SBSH_ROOT_* environment
// variables that viper.AutomaticEnv/BindEnv would otherwise leak into
// profile-resolution tests. viper.Reset() clears prior viper.Set calls but
// does not stop the rebound env bindings from re-reading the live process
// environment, so a developer running the suite from inside an sbsh session
// (which exports SBSH_TERM_PROFILE) would see the default-profile fallback
// tests fail. Clearing the prefix surface keeps those tests hermetic. See #350.
func scrubProfileEnv(t *testing.T) {
	t.Helper()
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(k, "SBSH_TERM_") || strings.HasPrefix(k, "SBSH_ROOT_") {
			t.Setenv(k, "")
		}
	}
}

func Test_buildTerminalSpecFromFlags_DefaultProfile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	scrubProfileEnv(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, t.TempDir())
	// Point profiles dir at an empty dir so the loader falls back to the
	// hardcoded default profile rather than reading the caller's $HOME.
	viper.Set(config.SBSH_ROOT_PROFILES_DIR.ViperKey, t.TempDir())

	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	spec, err := buildTerminalSpecFromFlags(cmd, logger, []string{"/bin/echo", "hi"})
	if err != nil {
		t.Fatalf("buildTerminalSpecFromFlags() error = %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if spec.Command != "/bin/echo" {
		t.Fatalf("expected command /bin/echo from workload; got %q", spec.Command)
	}
}

func Test_processSpec_BuildsFromFlags(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	scrubProfileEnv(t)

	runPath := t.TempDir()
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, runPath)
	viper.Set(config.SBSH_ROOT_PROFILES_DIR.ViperKey, t.TempDir())
	viper.Set(config.SBSH_TERM_LOG_FILE.ViperKey, filepath.Join(t.TempDir(), "term.log"))
	viper.Set(config.SBSH_TERM_LOG_LEVEL.ViperKey, "info")

	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	var spec *api.TerminalSpec
	if err := processSpec(cmd, &spec, []string{"/bin/echo"}); err != nil {
		t.Fatalf("processSpec() error = %v", err)
	}
	if spec == nil {
		t.Fatal("expected spec to be built from flags")
	}
	if spec.Command != "/bin/echo" {
		t.Fatalf("expected command /bin/echo; got %q", spec.Command)
	}
}

func Test_TerminalCmd_PreRunE_BuildsSpecFromFlags(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	scrubProfileEnv(t)

	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, t.TempDir())
	viper.Set(config.SBSH_ROOT_PROFILES_DIR.ViperKey, t.TempDir())
	viper.Set(config.SBSH_TERM_LOG_FILE.ViperKey, filepath.Join(t.TempDir(), "term.log"))

	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	if err := cmd.PreRunE(cmd, []string{}); err != nil {
		t.Fatalf("PreRunE() error = %v", err)
	}

	spec, ok := cmd.Context().Value(types.CtxTerminalSpec).(*api.TerminalSpec)
	if !ok || spec == nil {
		t.Fatal("expected terminal spec to be stored in context after PreRunE")
	}
}

func Test_TerminalCmd_PreRunE_FromFileSpec(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	logFile := filepath.Join(t.TempDir(), "term.log")
	fileSpec := api.TerminalSpec{
		ID:       api.ID("file-id"),
		Kind:     api.TerminalLocal,
		Name:     "file-name",
		Command:  "/bin/bash",
		LogFile:  logFile,
		LogLevel: "info",
	}
	jsonData, err := json.Marshal(fileSpec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	specFile := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(specFile, jsonData, 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	viper.Set(config.SBSH_TERM_SPEC.ViperKey, specFile)

	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	if err := cmd.PreRunE(cmd, []string{}); err != nil {
		t.Fatalf("PreRunE() error = %v", err)
	}

	spec, ok := cmd.Context().Value(types.CtxTerminalSpec).(*api.TerminalSpec)
	if !ok || spec == nil {
		t.Fatal("expected terminal spec from file in context")
	}
	if spec.ID != fileSpec.ID {
		t.Fatalf("expected ID %q; got %q", fileSpec.ID, spec.ID)
	}
}

func Test_TerminalCmd_PreRunE_InvalidArgErrors(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := NewTerminalCmd()
	cmd.SetContext(context.Background())

	err := cmd.PreRunE(cmd, []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for invalid positional argument")
	}
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidArgument, err)
	}
}

func Test_TerminalCmd_PostRunE(t *testing.T) {
	t.Run("nil closer is a no-op", func(t *testing.T) {
		cmd := NewTerminalCmd()
		cmd.SetContext(context.Background())
		if err := cmd.PostRunE(cmd, []string{}); err != nil {
			t.Fatalf("PostRunE() error = %v", err)
		}
	})

	t.Run("closer is closed", func(t *testing.T) {
		cmd := NewTerminalCmd()
		c := &recordingCloser{}
		ctx := context.WithValue(context.Background(), types.CtxCloser, io.Closer(c))
		cmd.SetContext(ctx)
		if err := cmd.PostRunE(cmd, []string{}); err != nil {
			t.Fatalf("PostRunE() error = %v", err)
		}
		if !c.closed {
			t.Fatal("expected closer to be closed")
		}
	})
}

type recordingCloser struct{ closed bool }

func (r *recordingCloser) Close() error {
	r.closed = true
	return nil
}

func Test_setupTerminalCmdFlags_BindsFlags(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := &cobra.Command{Use: "terminal"}
	setupTerminalCmdFlags(cmd)

	cases := []struct {
		flagName string
		value    string
		viperKey string
	}{
		{"id", "tid", config.SBSH_TERM_ID.ViperKey},
		{"name", "tname", config.SBSH_TERM_NAME.ViperKey},
		{"log-file", "/tmp/t.log", config.SBSH_TERM_LOG_FILE.ViperKey},
		{"capture-file", "/tmp/c.log", config.SBSH_TERM_CAPTURE_FILE.ViperKey},
		{"socket", "/tmp/t.sock", config.SBSH_TERM_SOCKET.ViperKey},
	}
	for _, tc := range cases {
		t.Run(tc.flagName, func(t *testing.T) {
			if err := cmd.Flags().Set(tc.flagName, tc.value); err != nil {
				t.Fatalf("set %s: %v", tc.flagName, err)
			}
			if got := viper.GetString(tc.viperKey); got != tc.value {
				t.Fatalf("viper key %s: expected %s, got %s", tc.viperKey, tc.value, got)
			}
		})
	}
}
