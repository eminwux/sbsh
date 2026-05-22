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

package sbsh

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/client"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestRunTerminal_ErrContextDone(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				// default: succeed without doing anything
				return nil
			},
			WaitReadyFunc: func() error {
				// default: succeed immediately
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				// default: succeed immediately
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(ctx, cancel, logger, ctrl, doc) // will block until ctx.Done()
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
		t.Fatal("timeout waiting for runClient to return after close")
	}
}

func TestRunTerminal_ErrWaitOnReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	expectedErr := errdefs.ErrStartRPCServer
	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				return nil
			},
			WaitReadyFunc: func() error {
				return expectedErr
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}
	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(ctx, cancel, logger, ctrl, doc)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrWaitOnReady) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)
		}
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", expectedErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runClient to return")
	}
}

func TestRunTerminal_ErrContextDoneWithWaitCloseError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	waitCloseErr := errdefs.ErrOnClose
	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				// Block to allow context cancellation
				time.Sleep(100 * time.Millisecond)
				return nil
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return waitCloseErr
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(ctx, cancel, logger, ctrl, doc)
	}()

	// Give Run() time to set ready, then cancel context
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrContextDone) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
		}
		if !errors.Is(err, errdefs.ErrWaitOnClose) {
			t.Fatalf("expected wrapped '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
		}
		if !errors.Is(err, waitCloseErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", waitCloseErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runClient to return")
	}
}

func TestRunTerminal_ErrChildExit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	runErr := errdefs.ErrRPCServerExited
	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				return runErr
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(ctx, cancel, logger, ctrl, doc)
	}()

	// Wait for Run() to complete and error to be handled
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrChildExit) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrChildExit, err)
		}
		if !errors.Is(err, runErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", runErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runClient to return")
	}
}

func TestRunTerminal_ErrChildExitWithWaitCloseError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	runErr := errdefs.ErrRPCServerExited
	waitCloseErr := errdefs.ErrOnClose
	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				return runErr
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return waitCloseErr
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(ctx, cancel, logger, ctrl, doc)
	}()

	// Wait for Run() to complete and error to be handled
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrChildExit) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrChildExit, err)
		}
		if !errors.Is(err, runErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", runErr, err)
		}
		if !errors.Is(err, errdefs.ErrWaitOnClose) {
			t.Fatalf("expected wrapped '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
		}
		if !errors.Is(err, waitCloseErr) {
			t.Fatalf("expected wrapped error '%v'; got: '%v'", waitCloseErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runClient to return")
	}
}

func TestRunTerminal_SuccessWithNilError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				return nil
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}

	done := make(chan error)
	defer close(done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(ctx, cancel, logger, ctrl, doc)
	}()

	// Wait for Run() to complete successfully
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error; got: '%v'", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runClient to return")
	}
}

func TestRunTerminal_SuccessWithContextCanceled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so Run() receives context.Canceled

	newClientController := func(_ context.Context) api.ClientController {
		return &client.ControllerTest{
			RunFunc: func(_ *api.ClientDoc) error {
				return ctx.Err() // Return context.Canceled
			},
			WaitReadyFunc: func() error {
				return nil
			},
			WaitCloseFunc: func() error {
				return nil
			},
			StartFunc: func() error {
				return nil
			},
		}
	}

	ctrl := newClientController(context.Background())

	t.Cleanup(func() {})

	// Define a new Client
	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:           api.ID(naming.RandomID()),
			LogFile:      "/tmp/sbsh-logs/s0",
			RunPath:      viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
			TerminalSpec: nil,
		},
	}

	done := make(chan error)
	defer close(done)

	runCtx, runCancel := context.WithCancel(context.Background())
	go func() {
		done <- runClient(runCtx, runCancel, logger, ctrl, doc)
	}()

	// Wait for Run() to complete with context.Canceled
	select {
	case err := <-done:
		// When errCh receives context.Canceled, runClient returns nil (line 414)
		if err != nil {
			t.Fatalf("expected nil (context.Canceled is ignored); got: '%v'", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runClient to return")
	}
}

func Test_setTerminalFlags_HappyPath(t *testing.T) {
	t.Setenv(config.SBSH_ROOT_TERM_PROFILE.EnvVar(), "")
	t.Cleanup(func() {
		viper.Reset()
	})

	flagCases := []struct {
		name     string
		flagName string
		value    string
		viperKey string
	}{
		{
			name:     "terminal-id",
			flagName: "terminal-id",
			value:    "term-id-123",
			viperKey: config.SBSH_ROOT_TERM_ID.ViperKey,
		},
		{
			name:     "terminal-command",
			flagName: "terminal-command",
			value:    "/usr/bin/zsh",
			viperKey: config.SBSH_ROOT_TERM_COMMAND.ViperKey,
		},
		{
			name:     "terminal-name",
			flagName: "terminal-name",
			value:    "dev-shell",
			viperKey: config.SBSH_ROOT_TERM_NAME.ViperKey,
		},
		{
			name:     "terminal-capture-file",
			flagName: "terminal-capture-file",
			value:    "/tmp/capture.log",
			viperKey: config.SBSH_ROOT_TERM_CAPTURE_FILE.ViperKey,
		},
		{
			name:     "terminal-log-file",
			flagName: "terminal-log-file",
			value:    "/tmp/term.log",
			viperKey: config.SBSH_ROOT_TERM_LOG_FILE.ViperKey,
		},
		{
			name:     "terminal-log-level",
			flagName: "terminal-log-level",
			value:    "debug",
			viperKey: config.SBSH_ROOT_TERM_LOG_LEVEL.ViperKey,
		},
		{
			name:     "terminal-profile",
			flagName: "terminal-profile",
			value:    "default",
			viperKey: config.SBSH_ROOT_TERM_PROFILE.ViperKey,
		},
		{
			name:     "terminal-socket",
			flagName: "terminal-socket",
			value:    "/tmp/term.sock",
			viperKey: config.SBSH_ROOT_TERM_SOCKET.ViperKey,
		},
	}

	for _, tc := range flagCases {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()

			rootCmd := &cobra.Command{Use: "sbsh"}
			if err := setTerminalFlags(rootCmd); err != nil {
				t.Fatalf("setTerminalFlags() error = %v", err)
			}

			if err := rootCmd.Flags().Set(tc.flagName, tc.value); err != nil {
				t.Fatalf("failed to set flag %s: %v", tc.flagName, err)
			}

			if got := viper.GetString(tc.viperKey); got != tc.value {
				t.Fatalf("viper key %s: expected %s, got %s", tc.viperKey, tc.value, got)
			}
		})
	}
}

func TestNewSbshRootCmd_WiresSubcommandsAndFlags(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	if rootCmd.Use != "sbsh" {
		t.Fatalf("Use = %q, want %q", rootCmd.Use, "sbsh")
	}

	wantSubcommands := []string{"version", "terminal", "autocomplete"}
	for _, name := range wantSubcommands {
		found := false
		for _, c := range rootCmd.Commands() {
			if c.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected subcommand %q to be registered", name)
		}
	}

	for _, flag := range []string{"run-path", "config", "profiles-dir"} {
		if rootCmd.PersistentFlags().Lookup(flag) == nil {
			t.Fatalf("expected persistent flag %q", flag)
		}
	}
	for _, flag := range []string{"id", "socket", "detach", "terminal-id", "terminal-profile"} {
		if rootCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag %q", flag)
		}
	}
}

func Test_setClientFlags_HappyPath(t *testing.T) {
	t.Cleanup(viper.Reset)

	flagCases := []struct {
		flagName string
		value    string
		viperKey string
		isBool   bool
	}{
		{"id", "client-123", config.SBSH_CLIENT_ID.ViperKey, false},
		{"socket", "/tmp/client.sock", config.SBSH_CLIENT_SOCKET.ViperKey, false},
		{"log-file", "/tmp/client.log", config.SBSH_CLIENT_LOG_FILE.ViperKey, false},
		{"log-level", "debug", config.SBSH_CLIENT_LOG_LEVEL.ViperKey, false},
		{"detach", "true", config.SBSH_CLIENT_DETACH.ViperKey, true},
		{"disable-detach", "true", config.SBSH_CLIENT_DISABLE_DETACH_KEYSTROKE.ViperKey, true},
	}

	for _, tc := range flagCases {
		t.Run(tc.flagName, func(t *testing.T) {
			viper.Reset()
			rootCmd := &cobra.Command{Use: "sbsh"}
			if err := setClientFlags(rootCmd); err != nil {
				t.Fatalf("setClientFlags() error = %v", err)
			}
			if err := rootCmd.Flags().Set(tc.flagName, tc.value); err != nil {
				t.Fatalf("set flag %s: %v", tc.flagName, err)
			}
			if tc.isBool {
				if !viper.GetBool(tc.viperKey) {
					t.Fatalf("viper key %s expected true", tc.viperKey)
				}
				return
			}
			if got := viper.GetString(tc.viperKey); got != tc.value {
				t.Fatalf("viper key %s: expected %s, got %s", tc.viperKey, tc.value, got)
			}
		})
	}
}

func Test_setPersistentLoggingFlags_HappyPath(t *testing.T) {
	t.Cleanup(viper.Reset)

	flagCases := []struct {
		flagName string
		value    string
		viperKey string
	}{
		{"run-path", "/tmp/run", config.SBSH_ROOT_RUN_PATH.ViperKey},
		{"config", "/tmp/config.yaml", config.SBSH_ROOT_CONFIG_FILE.ViperKey},
		{"profiles-dir", "/tmp/profiles.d", config.SBSH_ROOT_PROFILES_DIR.ViperKey},
	}

	for _, tc := range flagCases {
		t.Run(tc.flagName, func(t *testing.T) {
			viper.Reset()
			rootCmd := &cobra.Command{Use: "sbsh"}
			if err := setPersistentLoggingFlags(rootCmd); err != nil {
				t.Fatalf("setPersistentLoggingFlags() error = %v", err)
			}
			if err := rootCmd.PersistentFlags().Set(tc.flagName, tc.value); err != nil {
				t.Fatalf("set flag %s: %v", tc.flagName, err)
			}
			if got := viper.GetString(tc.viperKey); got != tc.value {
				t.Fatalf("viper key %s: expected %s, got %s", tc.viperKey, tc.value, got)
			}
		})
	}
}

func Test_readRootGIDFlag(t *testing.T) {
	const flagName = "terminal-socket-gid"
	const envName = "SBSH_TEST_ROOT_GID"

	t.Run("flag wins", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		if err := cmd.Flags().Set(flagName, "55"); err != nil {
			t.Fatalf("set: %v", err)
		}
		if got := readRootGIDFlag(cmd, flagName, envName); got == nil || *got != 55 {
			t.Fatalf("expected 55; got %v", got)
		}
	})

	t.Run("env fallback", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		t.Setenv(envName, "9")
		if got := readRootGIDFlag(cmd, flagName, envName); got == nil || *got != 9 {
			t.Fatalf("expected 9; got %v", got)
		}
	})

	t.Run("nil when unset", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int(flagName, -1, "")
		t.Setenv(envName, "")
		if got := readRootGIDFlag(cmd, flagName, envName); got != nil {
			t.Fatalf("expected nil; got %v", got)
		}
	})
}

func Test_readRootGIDWrappers(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	if err := rootCmd.Flags().Set("terminal-socket-gid", "21"); err != nil {
		t.Fatalf("set socket-gid: %v", err)
	}
	if err := rootCmd.Flags().Set("terminal-capture-gid", "22"); err != nil {
		t.Fatalf("set capture-gid: %v", err)
	}
	if err := rootCmd.Flags().Set("terminal-log-file-gid", "23"); err != nil {
		t.Fatalf("set log-file-gid: %v", err)
	}

	if got := readRootSocketGID(rootCmd); got == nil || *got != 21 {
		t.Fatalf("readRootSocketGID: expected 21; got %v", got)
	}
	if got := readRootCaptureGID(rootCmd); got == nil || *got != 22 {
		t.Fatalf("readRootCaptureGID: expected 22; got %v", got)
	}
	if got := readRootLogFileGID(rootCmd); got == nil || *got != 23 {
		t.Fatalf("readRootLogFileGID: expected 23; got %v", got)
	}
}

func Test_LoadConfig_Defaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	missingCfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), missingCfg)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	t.Setenv(config.SBSH_ROOT_RUN_PATH.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_PROFILES_DIR.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_LOG_LEVEL.EnvVar(), "")

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if got, want := viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey), config.DefaultRunPath(); got != want {
		t.Fatalf("run path = %q, want %q", got, want)
	}
	if got := viper.GetString(config.SBSH_ROOT_LOG_LEVEL.ViperKey); got != "info" {
		t.Fatalf("log level = %q, want info", got)
	}
}

func Test_LoadConfig_ConfigurationDoc(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `apiVersion: sbsh/v1beta1
kind: Configuration
metadata:
  name: default
spec:
  runPath: /tmp/sbsh-root-test-run
  profilesDir: /tmp/sbsh-root-test-profiles.d
  logLevel: debug
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), cfgPath)
	t.Setenv(config.SBSH_ROOT_RUN_PATH.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_PROFILES_DIR.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_LOG_LEVEL.EnvVar(), "")
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got, want := viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey), "/tmp/sbsh-root-test-run"; got != want {
		t.Fatalf("run path = %q, want %q", got, want)
	}
	if got, want := viper.GetString(config.SBSH_ROOT_PROFILES_DIR.ViperKey), "/tmp/sbsh-root-test-profiles.d"; got != want {
		t.Fatalf("profiles dir = %q, want %q", got, want)
	}
	if got, want := viper.GetString(config.SBSH_ROOT_LOG_LEVEL.ViperKey), "debug"; got != want {
		t.Fatalf("log level = %q, want %q", got, want)
	}
}

func Test_LoadConfig_EmptyConfigUsesDefault(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// No config file set anywhere -> LoadConfig falls back to
	// config.DefaultConfigFile(), which (absent on this host) loads as a
	// nil doc and the built-in defaults apply.
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_RUN_PATH.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_PROFILES_DIR.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_LOG_LEVEL.EnvVar(), "")

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got, want := viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey), config.DefaultRunPath(); got != want {
		t.Fatalf("run path = %q, want %q", got, want)
	}
}

func Test_LoadConfig_MalformedConfigErrors(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("\tnot: [valid"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), cfgPath)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()

	if err := LoadConfig(); err == nil {
		t.Fatal("expected error for malformed config file")
	}
}

func Test_RootCmd_PreRunE_SetupLoggerError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// Point the client log file at a path whose parent is a regular file so
	// the logger's MkdirAll/Open fails, exercising PreRunE's error return.
	parent := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	viper.Set(config.SBSH_CLIENT_LOG_FILE.ViperKey, filepath.Join(parent, "sub", "log"))

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	rootCmd.SetContext(context.Background())

	if err := rootCmd.PreRunE(rootCmd, []string{}); err == nil {
		t.Fatal("expected error when log file path is unwritable")
	}
}

func Test_detachSelf_AlreadyDetached(t *testing.T) {
	// SB_DETACHED=1 short-circuits before any re-exec, so the call is a
	// safe no-op (no os.Exit, no child process).
	t.Setenv("SB_DETACHED", "1")
	detachSelf()
}

func Test_RootCmd_RunE_DetachPath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// SB_DETACHED=1 makes detachSelf a no-op, so RunE's detach branch
	// returns nil without re-exec or os.Exit.
	t.Setenv("SB_DETACHED", "1")
	viper.Set(config.SBSH_CLIENT_DETACH.ViperKey, true)

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	rootCmd.SetContext(ctx)

	if err := rootCmd.RunE(rootCmd, []string{}); err != nil {
		t.Fatalf("RunE() detach path error = %v", err)
	}
}

func Test_RootCmd_RunE_MissingLogger(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	rootCmd.SetContext(context.Background())

	if err := rootCmd.RunE(rootCmd, []string{}); err == nil {
		t.Fatal("expected error when logger is missing from context")
	}
}

func Test_RootCmd_PersistentPreRunE(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	missingCfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), missingCfg)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	t.Setenv(config.SBSH_ROOT_RUN_PATH.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_PROFILES_DIR.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_LOG_LEVEL.EnvVar(), "")

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	rootCmd.SetContext(context.Background())

	if err := rootCmd.PersistentPreRunE(rootCmd, []string{}); err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
}

func Test_RootCmd_PreRunE_DerivesDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set(config.SBSH_ROOT_RUN_PATH.ViperKey, t.TempDir())

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	rootCmd.SetContext(context.Background())

	if err := rootCmd.PreRunE(rootCmd, []string{}); err != nil {
		t.Fatalf("PreRunE() error = %v", err)
	}

	if got := viper.GetString(config.SBSH_CLIENT_ID.ViperKey); got == "" {
		t.Fatal("expected a generated client ID")
	}
}

func Test_RootCmd_RunE_SpecBuildError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// No run path -> BuildTerminalSpecFromProfile rejects, exercising RunE's
	// logging + spec-param assembly and the build-error return.
	viper.Set(config.SBSH_CLIENT_DETACH.ViperKey, false)

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rootCmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	if err := rootCmd.RunE(rootCmd, []string{}); err == nil {
		t.Fatal("expected spec-build error when run path is empty")
	}
}

func Test_setAutoCompleteProfile_CompletionFunc(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv(config.SBSH_ROOT_PROFILES_DIR.EnvVar(), t.TempDir())

	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}

	fn, ok := rootCmd.GetFlagCompletionFunc("terminal-profile")
	if !ok || fn == nil {
		t.Fatal("expected a registered completion func for terminal-profile")
	}

	_, directive := fn(rootCmd, []string{}, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("expected ShellCompDirectiveNoFileComp; got %v", directive)
	}
}

func Test_RootCmd_PostRunE(t *testing.T) {
	rootCmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd() error = %v", err)
	}

	t.Run("nil closer", func(t *testing.T) {
		rootCmd.SetContext(context.Background())
		if err := rootCmd.PostRunE(rootCmd, []string{}); err != nil {
			t.Fatalf("PostRunE() error = %v", err)
		}
	})

	t.Run("closer closed", func(t *testing.T) {
		c := &rootRecordingCloser{}
		ctx := context.WithValue(context.Background(), types.CtxCloser, io.Closer(c))
		rootCmd.SetContext(ctx)
		if err := rootCmd.PostRunE(rootCmd, []string{}); err != nil {
			t.Fatalf("PostRunE() error = %v", err)
		}
		if !c.closed {
			t.Fatal("expected closer to be closed")
		}
	})
}

type rootRecordingCloser struct{ closed bool }

func (r *rootRecordingCloser) Close() error {
	r.closed = true
	return nil
}

func Test_setTerminalFlags_ProfileEnvBinding(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
	})

	envValue := "profile-from-env"
	t.Setenv(config.SBSH_ROOT_TERM_PROFILE.EnvVar(), envValue)

	rootCmd := &cobra.Command{Use: "sbsh"}
	if err := setTerminalFlags(rootCmd); err != nil {
		t.Fatalf("setTerminalFlags() error = %v", err)
	}

	if got := viper.GetString(config.SBSH_ROOT_TERM_PROFILE.ViperKey); got != envValue {
		t.Fatalf("expected env value %s for key %s, got %s", envValue, config.SBSH_ROOT_TERM_PROFILE.ViperKey, got)
	}
}
