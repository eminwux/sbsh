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

package sb

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/viper"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeCloser struct{ closed bool }

func (f *fakeCloser) Close() error {
	f.closed = true
	return nil
}

func Test_NewSbRootCmd_Construct(t *testing.T) {
	t.Cleanup(viper.Reset)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "sb" {
		t.Fatalf("expected Use \"sb\"; got %q", cmd.Use)
	}
}

func Test_PersistentPreRunE_NonVerbose(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()

	missingCfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), missingCfg)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	viper.Set(config.SB_ROOT_VERBOSE.ViperKey, false)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.Background())

	if err := cmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
}

func Test_PersistentPreRunE_Verbose(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()

	missingCfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), missingCfg)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	viper.Set(config.SB_ROOT_VERBOSE.ViperKey, true)
	viper.Set(config.SB_ROOT_LOG_LEVEL.ViperKey, "")

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.Background())

	if err := cmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
}

func Test_PersistentPreRunE_Verbose_ConfigError(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()

	bad := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(bad, []byte("\tnot: [valid yaml"), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), bad)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	viper.Set(config.SB_ROOT_VERBOSE.ViperKey, true)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.Background())

	err = cmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrConfig) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrConfig, err)
	}
}

func Test_PersistentPreRunE_NonVerbose_ConfigError(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()

	bad := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(bad, []byte("\tnot: [valid yaml"), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), bad)
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	viper.Set(config.SB_ROOT_VERBOSE.ViperKey, false)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.Background())

	// With --verbose off the logger is nil; the error path must not panic.
	err = cmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrConfig) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrConfig, err)
	}
}

func Test_RunE_LoggerNotFound(t *testing.T) {
	t.Cleanup(viper.Reset)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.Background())

	err = cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_RunE_Help(t *testing.T) {
	t.Cleanup(viper.Reset)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, discardLogger()))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func Test_PostRunE_NoCloser(t *testing.T) {
	t.Cleanup(viper.Reset)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	cmd.SetContext(context.Background())

	if err := cmd.PostRunE(cmd, nil); err != nil {
		t.Fatalf("PostRunE() error = %v", err)
	}
}

func Test_PostRunE_WithCloser(t *testing.T) {
	t.Cleanup(viper.Reset)

	cmd, err := NewSbRootCmd()
	if err != nil {
		t.Fatalf("NewSbRootCmd() error = %v", err)
	}
	fc := &fakeCloser{}
	cmd.SetContext(context.WithValue(context.Background(), types.CtxCloser, io.Closer(fc)))

	if err := cmd.PostRunE(cmd, nil); err != nil {
		t.Fatalf("PostRunE() error = %v", err)
	}
	if !fc.closed {
		t.Fatal("expected closer to be closed")
	}
}

func Test_LoadConfig_DefaultConfigFile(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), "")
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}
