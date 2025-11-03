//go:build integration
// +build integration

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

package get

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Helper Functions

func setupTestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func setupTestCmd(t *testing.T, logger *slog.Logger) (*cobra.Command, context.Context) {
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	// Initialize flags for getTerminal/getSupervisor
	cmd.Flags().String("run-path", "", "Run path")
	return cmd, ctx
}

func captureStdout(fn func()) (string, error) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, _ = io.Copy(&buf, r)
		wg.Done()
	}()

	fn()

	w.Close()
	os.Stdout = oldStdout
	wg.Wait()
	r.Close()

	return buf.String(), nil
}

func createTestProfileFile(t *testing.T, dir string, profilesYAML string) string {
	profilesFile := filepath.Join(dir, "profiles.yaml")
	if err := os.WriteFile(profilesFile, []byte(profilesYAML), 0o644); err != nil {
		t.Fatalf("failed to create profiles file: %v", err)
	}
	return profilesFile
}

func createTestTerminalMetadata(t *testing.T, runPath string, id string, name string, state api.SessionStatusMode) {
	// Create directory: runPath/sessions/{id}/
	sessDir := filepath.Join(runPath, "sessions", id)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	// Create metadata.json with SessionMetadata structure
	metadata := api.SessionMetadata{
		Spec: api.SessionSpec{
			ID:          api.ID(id),
			Kind:        api.SessionLocal,
			Name:        name,
			Command:     "/bin/bash",
			CommandArgs: []string{"-i"},
			EnvInherit:  true,
			RunPath:     runPath,
		},
		Status: api.SessionStatus{
			Pid:   12345,
			Tty:   "/dev/pts/0",
			State: state,
		},
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}
	metaPath := filepath.Join(sessDir, "metadata.json")
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}
}

func createTestSupervisorMetadata(
	t *testing.T,
	runPath string,
	id string,
	name string,
	state api.SupervisorStatusMode,
) {
	// Create directory: runPath/supervisors/{id}/
	supDir := filepath.Join(runPath, "supervisors", id)
	if err := os.MkdirAll(supDir, 0o755); err != nil {
		t.Fatalf("failed to create supervisor dir: %v", err)
	}

	// Create metadata.json with SupervisorMetadata structure
	metadata := api.SupervisorMetadata{
		Spec: api.SupervisorSpec{
			ID:   api.ID(id),
			Kind: api.RunNewSession,
			Name: name,
		},
		Status: api.SupervisorStatus{
			Pid:   12345,
			State: state,
		},
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}
	metaPath := filepath.Join(supDir, "metadata.json")
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}
}

// Profile Integration Tests

func Test_FetchProfileNames_Integration(t *testing.T) {
	logger := setupTestLogger(t)
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	t.Run("success with multiple profiles", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: k8s-default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: terraform-prd
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		names, err := fetchProfileNames(ctx, profilesFile, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 3 {
			t.Fatalf("expected 3 names, got %d", len(names))
		}
		expected := []string{"default", "k8s-default", "terraform-prd"}
		for _, exp := range expected {
			found := false
			for _, name := range names {
				if name == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected name %q not found in results", exp)
			}
		}
	})

	t.Run("success with prefix filtering", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: k8s-default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: terraform-prd
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		names, err := fetchProfileNames(ctx, profilesFile, "k8s")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 1 {
			t.Fatalf("expected 1 name, got %d", len(names))
		}
		if names[0] != "k8s-default" {
			t.Errorf("expected 'k8s-default', got %q", names[0])
		}
	})

	t.Run("success with empty prefix", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: k8s-default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: terraform-prd
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		names, err := fetchProfileNames(ctx, profilesFile, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 3 {
			t.Fatalf("expected 3 names, got %d", len(names))
		}
	})

	t.Run("error when profiles file doesn't exist", func(t *testing.T) {
		_, err := fetchProfileNames(ctx, "/nonexistent/path/profiles.yaml", "")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})

	t.Run("error when profiles file is invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilesFile := filepath.Join(tmpDir, "profiles.yaml")
		if err := os.WriteFile(profilesFile, []byte("invalid: yaml: content: [unclosed"), 0o644); err != nil {
			t.Fatalf("failed to create invalid YAML file: %v", err)
		}

		_, err := fetchProfileNames(ctx, profilesFile, "")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})

	t.Run("success with empty profiles file", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilesFile := createTestProfileFile(t, tmpDir, "")

		names, err := fetchProfileNames(ctx, profilesFile, "")
		// AutoCompleteListProfileNames returns an error "no profiles found" for empty file
		if err != nil {
			// This is expected behavior - empty profiles file results in error
			return
		}
		// If no error, should return empty slice
		if names == nil {
			t.Fatal("expected empty slice, got nil")
		}
		if len(names) != 0 {
			t.Fatalf("expected empty slice, got %d items", len(names))
		}
	})
}

func Test_ListProfiles_Integration(t *testing.T) {
	logger := setupTestLogger(t)

	t.Run("success with valid profiles", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: k8s-default
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		viper.Set(config.PROFILES_FILE.ViperKey, profilesFile)
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
		}()

		output, err := captureStdout(func() {
			_ = listProfiles(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "default") {
			t.Errorf("output should contain 'default', got: %s", output)
		}
		if !strings.Contains(output, "k8s-default") {
			t.Errorf("output should contain 'k8s-default', got: %s", output)
		}
	})

	t.Run("success with empty profiles file", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		tmpDir := t.TempDir()
		profilesFile := createTestProfileFile(t, tmpDir, "")

		viper.Set(config.PROFILES_FILE.ViperKey, profilesFile)
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
		}()

		output, err := captureStdout(func() {
			_ = listProfiles(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "no profiles found") {
			t.Errorf("output should contain 'no profiles found', got: %s", output)
		}
	})

	t.Run("error when profiles file path is invalid", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)

		viper.Set(config.PROFILES_FILE.ViperKey, "/nonexistent/path/profiles.yaml")
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
		}()

		err := listProfiles(cmd, []string{})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})
}

func Test_GetProfile_Integration(t *testing.T) {
	logger := setupTestLogger(t)

	t.Run("success with default format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		viper.Set(config.PROFILES_FILE.ViperKey, profilesFile)
		viper.Set(outputFormatProfilesInput, "")
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
			viper.Set(outputFormatProfilesInput, "")
		}()

		output, err := captureStdout(func() {
			_ = getProfile(cmd, []string{"default"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if len(output) == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("success with json format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		viper.Set(config.PROFILES_FILE.ViperKey, profilesFile)
		viper.Set(outputFormatProfilesInput, "json")
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
			viper.Set(outputFormatProfilesInput, "")
		}()

		output, err := captureStdout(func() {
			_ = getProfile(cmd, []string{"default"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		var result api.SessionProfileDoc
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v, output: %s", err, output)
		}
		if result.Metadata.Name != "default" {
			t.Errorf("expected name 'default', got %q", result.Metadata.Name)
		}
	})

	t.Run("success with yaml format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		viper.Set(config.PROFILES_FILE.ViperKey, profilesFile)
		viper.Set(outputFormatProfilesInput, "yaml")
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
			viper.Set(outputFormatProfilesInput, "")
		}()

		output, err := captureStdout(func() {
			_ = getProfile(cmd, []string{"default"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		var result api.SessionProfileDoc
		if err := yaml.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("output is not valid YAML: %v, output: %s", err, output)
		}
		if result.Metadata.Name != "default" {
			t.Errorf("expected name 'default', got %q", result.Metadata.Name)
		}
	})

	t.Run("error when profile doesn't exist", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		tmpDir := t.TempDir()
		profilesYAML := `apiVersion: sbsh/v1beta1
kind: SessionProfile
metadata:
  name: default
spec:
  shell:
    cmd: /bin/bash
`
		profilesFile := createTestProfileFile(t, tmpDir, profilesYAML)

		viper.Set(config.PROFILES_FILE.ViperKey, profilesFile)
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
		}()

		err := getProfile(cmd, []string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})

	t.Run("error when profiles file doesn't exist", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)

		viper.Set(config.PROFILES_FILE.ViperKey, "/nonexistent/path/profiles.yaml")
		defer func() {
			viper.Set(config.PROFILES_FILE.ViperKey, "")
		}()

		err := getProfile(cmd, []string{"default"})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})
}

// Terminal Integration Tests

func Test_FetchTerminalNames_Integration(t *testing.T) {
	logger := setupTestLogger(t)
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	t.Run("success with multiple terminals", func(t *testing.T) {
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)
		createTestTerminalMetadata(t, runPath, "term2", "brave_gandalf", api.Ready)
		createTestTerminalMetadata(t, runPath, "term3", "silent_aragorn", api.Ready)

		names, err := fetchTerminalNames(ctx, runPath, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 3 {
			t.Fatalf("expected 3 names, got %d", len(names))
		}
		expected := []string{"twilight_anarion", "brave_gandalf", "silent_aragorn"}
		for _, exp := range expected {
			found := false
			for _, name := range names {
				if name == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected name %q not found in results", exp)
			}
		}
	})

	t.Run("success with prefix filtering", func(t *testing.T) {
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)
		createTestTerminalMetadata(t, runPath, "term2", "brave_gandalf", api.Ready)
		createTestTerminalMetadata(t, runPath, "term3", "silent_aragorn", api.Ready)

		names, err := fetchTerminalNames(ctx, runPath, "twi")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 1 {
			t.Fatalf("expected 1 name, got %d", len(names))
		}
		if names[0] != "twilight_anarion" {
			t.Errorf("expected 'twilight_anarion', got %q", names[0])
		}
	})

	t.Run("success with empty runPath", func(t *testing.T) {
		emptyDir := t.TempDir()

		names, err := fetchTerminalNames(ctx, emptyDir, "")
		// AutoCompleteListTerminalNames may return error for empty directory
		if err != nil {
			// This is acceptable - empty directory results in error
			return
		}
		if names == nil {
			t.Fatal("expected empty slice, got nil")
		}
		if len(names) != 0 {
			t.Fatalf("expected empty slice, got %d items", len(names))
		}
	})
}

func Test_ResolveTerminalNameToID_Integration(t *testing.T) {
	logger := setupTestLogger(t)
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	t.Run("success: resolves name to ID correctly", func(t *testing.T) {
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term123", "twilight_anarion", api.Ready)

		id, err := ResolveTerminalNameToID(ctx, logger, runPath, "twilight_anarion")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "term123" {
			t.Errorf("expected ID 'term123', got %q", id)
		}
	})

	t.Run("error: terminal name not found", func(t *testing.T) {
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term123", "twilight_anarion", api.Ready)

		_, err := ResolveTerminalNameToID(ctx, logger, runPath, "nonexistent")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if !errors.Is(err, errdefs.ErrTerminalNotFound) {
			t.Errorf("expected ErrTerminalNotFound, got: %v", err)
		}
	})

	t.Run("error: no terminals found", func(t *testing.T) {
		runPath := t.TempDir()

		_, err := ResolveTerminalNameToID(ctx, logger, runPath, "any-name")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		// When no terminals are found, it returns ErrTerminalNotFound (not ErrNoTerminalsFound)
		// because the check for nil terminals is never true (ScanTerminals returns empty slice)
		if !errors.Is(err, errdefs.ErrTerminalNotFound) {
			t.Errorf("expected ErrTerminalNotFound, got: %v", err)
		}
	})

	t.Run("error: invalid metadata file", func(t *testing.T) {
		runPath := t.TempDir()
		sessDir := filepath.Join(runPath, "sessions", "term123")
		if err := os.MkdirAll(sessDir, 0o755); err != nil {
			t.Fatalf("failed to create session dir: %v", err)
		}
		metaPath := filepath.Join(sessDir, "metadata.json")
		if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
			t.Fatalf("failed to write invalid metadata: %v", err)
		}

		_, err := ResolveTerminalNameToID(ctx, logger, runPath, "any-name")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})
}

func Test_ListTerminals_Integration(t *testing.T) {
	logger := setupTestLogger(t)

	t.Run("success with terminals", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)
		createTestTerminalMetadata(t, runPath, "term2", "brave_gandalf", api.Ready)

		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(listAllInput, false)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(listAllInput, false)
		}()

		output, err := captureStdout(func() {
			_ = listTerminals(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "twilight_anarion") {
			t.Errorf("output should contain 'twilight_anarion', got: %s", output)
		}
		if !strings.Contains(output, "brave_gandalf") {
			t.Errorf("output should contain 'brave_gandalf', got: %s", output)
		}
	})

	t.Run("success with --all flag", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)
		createTestTerminalMetadata(t, runPath, "term2", "brave_gandalf", api.Exited)

		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(listAllInput, true)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(listAllInput, false)
		}()

		output, err := captureStdout(func() {
			_ = listTerminals(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "twilight_anarion") {
			t.Errorf("output should contain 'twilight_anarion', got: %s", output)
		}
		if !strings.Contains(output, "brave_gandalf") {
			t.Errorf("output should contain 'brave_gandalf' (exited), got: %s", output)
		}
	})

	t.Run("success with empty directory", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()

		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(listAllInput, false)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(listAllInput, false)
		}()

		output, err := captureStdout(func() {
			_ = listTerminals(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "no active or inactive terminals found") {
			t.Errorf("output should contain 'no active or inactive terminals found', got: %s", output)
		}
	})
}

func Test_GetTerminal_Integration(t *testing.T) {
	logger := setupTestLogger(t)

	t.Run("success with default format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(outputFormat, "")
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(outputFormat, "")
		}()

		output, err := captureStdout(func() {
			_ = getTerminal(cmd, []string{"twilight_anarion"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if len(output) == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("success with json format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(outputFormat, "json")
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(outputFormat, "")
		}()

		output, err := captureStdout(func() {
			_ = getTerminal(cmd, []string{"twilight_anarion"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		var result api.SessionMetadata
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v, output: %s", err, output)
		}
		if result.Spec.Name != "twilight_anarion" {
			t.Errorf("expected name 'twilight_anarion', got %q", result.Spec.Name)
		}
	})

	t.Run("success with yaml format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestTerminalMetadata(t, runPath, "term1", "twilight_anarion", api.Ready)

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(outputFormat, "yaml")
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(outputFormat, "")
		}()

		output, err := captureStdout(func() {
			_ = getTerminal(cmd, []string{"twilight_anarion"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		var result api.SessionMetadata
		if err := yaml.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("output is not valid YAML: %v, output: %s", err, output)
		}
		if result.Spec.Name != "twilight_anarion" {
			t.Errorf("expected name 'twilight_anarion', got %q", result.Spec.Name)
		}
	})

	t.Run("error when terminal doesn't exist", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
		}()

		err := getTerminal(cmd, []string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})
}

// Supervisor Integration Tests

func Test_FetchSupervisorNames_Integration(t *testing.T) {
	logger := setupTestLogger(t)
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	t.Run("success with multiple supervisors", func(t *testing.T) {
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)
		createTestSupervisorMetadata(t, runPath, "sup2", "super_two", api.SupervisorReady)
		createTestSupervisorMetadata(t, runPath, "sup3", "super_three", api.SupervisorReady)

		names, err := fetchSupervisorNames(ctx, runPath, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 3 {
			t.Fatalf("expected 3 names, got %d", len(names))
		}
		expected := []string{"super_one", "super_two", "super_three"}
		for _, exp := range expected {
			found := false
			for _, name := range names {
				if name == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected name %q not found in results", exp)
			}
		}
	})

	t.Run("success with prefix filtering", func(t *testing.T) {
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)
		createTestSupervisorMetadata(t, runPath, "sup2", "super_two", api.SupervisorReady)
		createTestSupervisorMetadata(t, runPath, "sup3", "super_three", api.SupervisorReady)

		names, err := fetchSupervisorNames(ctx, runPath, "super_one")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 1 {
			t.Fatalf("expected 1 name, got %d", len(names))
		}
		if names[0] != "super_one" {
			t.Errorf("expected 'super_one', got %q", names[0])
		}
	})

	t.Run("success with empty runPath", func(t *testing.T) {
		emptyDir := t.TempDir()

		names, err := fetchSupervisorNames(ctx, emptyDir, "")
		// AutoCompleteListSupervisorNames may return error for empty directory
		if err != nil {
			// This is acceptable - empty directory results in error
			return
		}
		if names == nil {
			t.Fatal("expected empty slice, got nil")
		}
		if len(names) != 0 {
			t.Fatalf("expected empty slice, got %d items", len(names))
		}
	})
}

func Test_ResolveSupervisorNameToID_Integration(t *testing.T) {
	logger := setupTestLogger(t)
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	t.Run("success: resolves name to ID correctly", func(t *testing.T) {
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup123", "super_one", api.SupervisorReady)

		id, err := ResolveSupervisorNameToID(ctx, logger, runPath, "super_one")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "sup123" {
			t.Errorf("expected ID 'sup123', got %q", id)
		}
	})

	t.Run("error: supervisor name not found", func(t *testing.T) {
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup123", "super_one", api.SupervisorReady)

		_, err := ResolveSupervisorNameToID(ctx, logger, runPath, "nonexistent")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if !errors.Is(err, errdefs.ErrSupervisorNotFound) {
			t.Errorf("expected ErrSupervisorNotFound, got: %v", err)
		}
	})

	t.Run("error: no supervisors found", func(t *testing.T) {
		runPath := t.TempDir()

		_, err := ResolveSupervisorNameToID(ctx, logger, runPath, "any-name")
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		// When no supervisors are found, it returns ErrSupervisorNotFound (not ErrNoSupervisorsFound)
		// because the check for nil supervisors is never true (ScanSupervisors returns empty slice)
		if !errors.Is(err, errdefs.ErrSupervisorNotFound) {
			t.Errorf("expected ErrSupervisorNotFound, got: %v", err)
		}
	})
}

func Test_ListSupervisors_Integration(t *testing.T) {
	logger := setupTestLogger(t)

	t.Run("success with supervisors", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)
		createTestSupervisorMetadata(t, runPath, "sup2", "super_two", api.SupervisorReady)

		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(listAllSupervisorsInput, false)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(listAllSupervisorsInput, false)
		}()

		output, err := captureStdout(func() {
			_ = listSupervisors(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "super_one") {
			t.Errorf("output should contain 'super_one', got: %s", output)
		}
		if !strings.Contains(output, "super_two") {
			t.Errorf("output should contain 'super_two', got: %s", output)
		}
	})

	t.Run("success with --all flag", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)
		createTestSupervisorMetadata(t, runPath, "sup2", "super_two", api.SupervisorExited)

		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(listAllSupervisorsInput, true)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(listAllSupervisorsInput, false)
		}()

		output, err := captureStdout(func() {
			_ = listSupervisors(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "super_one") {
			t.Errorf("output should contain 'super_one', got: %s", output)
		}
		if !strings.Contains(output, "super_two") {
			t.Errorf("output should contain 'super_two' (exited), got: %s", output)
		}
	})

	t.Run("success with empty directory", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()

		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(listAllSupervisorsInput, false)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(listAllSupervisorsInput, false)
		}()

		output, err := captureStdout(func() {
			_ = listSupervisors(cmd, []string{})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if !strings.Contains(output, "no active or inactive supervisors found") {
			t.Errorf("output should contain 'no active or inactive supervisors found', got: %s", output)
		}
	})
}

func Test_GetSupervisor_Integration(t *testing.T) {
	logger := setupTestLogger(t)

	t.Run("success with default format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(outputFormatSupervisorInput, "")
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(outputFormatSupervisorInput, "")
		}()

		output, err := captureStdout(func() {
			_ = getSupervisor(cmd, []string{"super_one"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		if len(output) == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("success with json format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(outputFormatSupervisorInput, "json")
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(outputFormatSupervisorInput, "")
		}()

		output, err := captureStdout(func() {
			_ = getSupervisor(cmd, []string{"super_one"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		var result api.SupervisorMetadata
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v, output: %s", err, output)
		}
		if result.Spec.Name != "super_one" {
			t.Errorf("expected name 'super_one', got %q", result.Spec.Name)
		}
	})

	t.Run("success with yaml format", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()
		createTestSupervisorMetadata(t, runPath, "sup1", "super_one", api.SupervisorReady)

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		viper.Set(outputFormatSupervisorInput, "yaml")
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
			viper.Set(outputFormatSupervisorInput, "")
		}()

		output, err := captureStdout(func() {
			_ = getSupervisor(cmd, []string{"super_one"})
		})
		if err != nil {
			t.Fatalf("failed to capture stdout: %v", err)
		}

		var result api.SupervisorMetadata
		if err := yaml.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("output is not valid YAML: %v, output: %s", err, output)
		}
		if result.Spec.Name != "super_one" {
			t.Errorf("expected name 'super_one', got %q", result.Spec.Name)
		}
	})

	t.Run("error when supervisor doesn't exist", func(t *testing.T) {
		cmd, _ := setupTestCmd(t, logger)
		runPath := t.TempDir()

		cmd.Flags().Set("run-path", runPath)
		viper.Set(config.RUN_PATH.ViperKey, runPath)
		defer func() {
			viper.Set(config.RUN_PATH.ViperKey, "")
		}()

		err := getSupervisor(cmd, []string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
	})
}
