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

package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// unsetEnv removes the given env vars for the duration of the test, restoring
// their prior values on cleanup so production keys do not leak across tests.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		k := k
		if old, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { _ = os.Setenv(k, old) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(k) })
		}
		_ = os.Unsetenv(k)
	}
}

func TestApplyConfigurationDocEnv(t *testing.T) {
	t.Run("nil doc is a no-op", func(t *testing.T) {
		ApplyConfigurationDocEnv(nil)
	})

	t.Run("sets unset vars and skips empty values", func(t *testing.T) {
		keys := []string{
			SB_ROOT_RUN_PATH.EnvVar(), SBSH_ROOT_RUN_PATH.EnvVar(),
			SB_GET_PROFILES_DIR.EnvVar(), SBSH_ROOT_PROFILES_DIR.EnvVar(),
			SB_ROOT_LOG_LEVEL.EnvVar(), SBSH_ROOT_LOG_LEVEL.EnvVar(),
		}
		unsetEnv(t, keys...)

		// LogLevel left empty exercises the value == "" early return.
		doc := &api.ConfigurationDoc{
			Spec: api.ConfigurationSpec{
				RunPath:     "/var/run/sbsh",
				ProfilesDir: "/etc/sbsh/profiles.d",
			},
		}
		ApplyConfigurationDocEnv(doc)

		if got := os.Getenv(SB_ROOT_RUN_PATH.EnvVar()); got != "/var/run/sbsh" {
			t.Errorf("SB_RUN_PATH = %q, want /var/run/sbsh", got)
		}
		if got := os.Getenv(SBSH_ROOT_PROFILES_DIR.EnvVar()); got != "/etc/sbsh/profiles.d" {
			t.Errorf("SBSH_PROFILES_DIR = %q, want /etc/sbsh/profiles.d", got)
		}
		if _, present := os.LookupEnv(SB_ROOT_LOG_LEVEL.EnvVar()); present {
			t.Errorf("SB_LOG_LEVEL should remain unset for empty LogLevel")
		}
	})

	t.Run("preserves already-set vars", func(t *testing.T) {
		key := SB_ROOT_RUN_PATH.EnvVar()
		unsetEnv(t, key)
		t.Setenv(key, "user-supplied")

		ApplyConfigurationDocEnv(&api.ConfigurationDoc{
			Spec: api.ConfigurationSpec{RunPath: "/from/doc"},
		})

		if got := os.Getenv(key); got != "user-supplied" {
			t.Errorf("SB_RUN_PATH = %q, want user-supplied (precedence env > doc)", got)
		}
	})
}

func TestDefaultPaths(t *testing.T) {
	for _, fn := range []struct {
		name string
		got  string
	}{
		{"DefaultRunPath", DefaultRunPath()},
		{"DefaultProfilesDir", DefaultProfilesDir()},
		{"DefaultConfigFile", DefaultConfigFile()},
	} {
		if !strings.Contains(fn.got, ".sbsh") {
			t.Errorf("%s() = %q, want it to contain .sbsh", fn.name, fn.got)
		}
	}
}

func TestGetRunPathFromEnvAndFlags(t *testing.T) {
	const envVar = "SBSH_TEST_RUN_PATH"
	unsetEnv(t, envVar)

	t.Run("flag wins", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("run-path", "", "")
		_ = cmd.Flags().Set("run-path", "/from/flag")
		got, err := GetRunPathFromEnvAndFlags(cmd, envVar)
		if err != nil || got != "/from/flag" {
			t.Fatalf("got (%q, %v), want (/from/flag, nil)", got, err)
		}
	})

	t.Run("env when flag empty", func(t *testing.T) {
		t.Setenv(envVar, "/from/env")
		cmd := &cobra.Command{}
		cmd.Flags().String("run-path", "", "")
		got, err := GetRunPathFromEnvAndFlags(cmd, envVar)
		if err != nil || got != "/from/env" {
			t.Fatalf("got (%q, %v), want (/from/env, nil)", got, err)
		}
	})

	t.Run("default when flag and env empty", func(t *testing.T) {
		unsetEnv(t, envVar)
		cmd := &cobra.Command{}
		cmd.Flags().String("run-path", "", "")
		got, err := GetRunPathFromEnvAndFlags(cmd, envVar)
		if err != nil || got != DefaultRunPath() {
			t.Fatalf("got (%q, %v), want (%q, nil)", got, err, DefaultRunPath())
		}
	})
}

func TestGetProfilesDirFromEnvAndFlags(t *testing.T) {
	const envVar = "SBSH_TEST_PROFILES_DIR"
	unsetEnv(t, envVar)

	t.Run("flag wins", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("profiles-dir", "", "")
		_ = cmd.Flags().Set("profiles-dir", "/from/flag")
		got, err := GetProfilesDirFromEnvAndFlags(cmd, envVar)
		if err != nil || got != "/from/flag" {
			t.Fatalf("got (%q, %v), want (/from/flag, nil)", got, err)
		}
	})

	t.Run("env when flag empty", func(t *testing.T) {
		t.Setenv(envVar, "/from/env")
		cmd := &cobra.Command{}
		cmd.Flags().String("profiles-dir", "", "")
		got, err := GetProfilesDirFromEnvAndFlags(cmd, envVar)
		if err != nil || got != "/from/env" {
			t.Fatalf("got (%q, %v), want (/from/env, nil)", got, err)
		}
	})

	t.Run("default when flag and env empty", func(t *testing.T) {
		unsetEnv(t, envVar)
		cmd := &cobra.Command{}
		cmd.Flags().String("profiles-dir", "", "")
		got, err := GetProfilesDirFromEnvAndFlags(cmd, envVar)
		if err != nil || got != DefaultProfilesDir() {
			t.Fatalf("got (%q, %v), want (%q, nil)", got, err, DefaultProfilesDir())
		}
	})
}

func TestVarMethods(t *testing.T) {
	const envKey = "SBSH_TEST_VAR"
	const viperKey = "test/var"
	unsetEnv(t, envKey)

	v := DefineKV(envKey, viperKey, "fallback")
	if v.EnvKey() != envKey || v.EnvVar() != envKey {
		t.Fatalf("EnvKey/EnvVar = %q/%q, want %q", v.EnvKey(), v.EnvVar(), envKey)
	}
	if def, ok := v.DefaultValue(); !ok || def != "fallback" {
		t.Fatalf("DefaultValue = (%q, %v), want (fallback, true)", def, ok)
	}

	// default branch: no viper, no env.
	viper.Reset()
	t.Cleanup(viper.Reset)
	if got := v.ValueOrDefault(); got != "fallback" {
		t.Fatalf("ValueOrDefault default = %q, want fallback", got)
	}

	// env branch.
	t.Setenv(envKey, "from-env")
	if got := v.ValueOrDefault(); got != "from-env" {
		t.Fatalf("ValueOrDefault env = %q, want from-env", got)
	}

	// viper branch wins over env.
	viper.Set(viperKey, "from-viper")
	if got := v.ValueOrDefault(); got != "from-viper" {
		t.Fatalf("ValueOrDefault viper = %q, want from-viper", got)
	}

	if err := v.BindEnv(); err != nil {
		t.Fatalf("BindEnv() error = %v", err)
	}

	// Define without a viper key: ValueOrDefault skips viper, BindEnv is a no-op,
	// and an absent env with no default yields "".
	noViper := Define("SBSH_TEST_NOVIPER")
	unsetEnv(t, "SBSH_TEST_NOVIPER")
	if err := noViper.BindEnv(); err != nil {
		t.Fatalf("BindEnv() on empty viper key = %v, want nil", err)
	}
	if got := noViper.ValueOrDefault(); got != "" {
		t.Fatalf("ValueOrDefault no-default = %q, want empty", got)
	}

	// Set writes the OS env; SetDefault updates the default + viper default.
	if err := v.Set("written"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got := os.Getenv(envKey); got != "written" {
		t.Fatalf("after Set, env = %q, want written", got)
	}
	v.SetDefault("new-default")
	if def, ok := v.DefaultValue(); !ok || def != "new-default" {
		t.Fatalf("after SetDefault, DefaultValue = (%q, %v), want (new-default, true)", def, ok)
	}

	if got := KV(v, "x"); got != envKey+"=x" {
		t.Fatalf("KV = %q, want %q", got, envKey+"=x")
	}
}

const sampleProfile = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: alpha
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
`

func writeTerminalDoc(t *testing.T, runPath, id, name string, state api.TerminalStatusMode) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Metadata:   api.TerminalMetadata{Name: name},
		Spec:       api.TerminalSpec{ID: api.ID(id), Name: name, RunPath: runPath},
		Status:     api.TerminalStatus{State: state},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writeClientDoc(t *testing.T, runPath, id, name string, state api.ClientStatusMode) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.ClientsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	doc := api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Name: name},
		Spec:       api.ClientSpec{ID: api.ID(id), RunPath: runPath},
		Status:     api.ClientStatus{State: state},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestAutoCompleteListProfileNames(t *testing.T) {
	ctx := context.Background()

	t.Run("no profiles", func(t *testing.T) {
		if _, err := AutoCompleteListProfileNames(ctx, nil, t.TempDir()); err == nil {
			t.Fatal("expected error for empty profiles dir, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte(sampleProfile), 0o644); err != nil {
			t.Fatalf("write profile: %v", err)
		}
		names, err := AutoCompleteListProfileNames(ctx, nil, dir)
		if err != nil {
			t.Fatalf("AutoCompleteListProfileNames() error = %v", err)
		}
		if !slices.Contains(names, "alpha") {
			t.Fatalf("names = %v, want it to contain alpha", names)
		}
	})
}

func TestAutoCompleteListTerminalNamesAndIDs(t *testing.T) {
	ctx := context.Background()

	t.Run("empty run path yields no names", func(t *testing.T) {
		names, err := AutoCompleteListTerminalNames(ctx, nil, t.TempDir(), false)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(names) != 0 {
			t.Fatalf("names = %v, want empty", names)
		}
	})

	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-active", "active", api.Ready)
	writeTerminalDoc(t, runPath, "id-exited", "exited", api.Exited)

	t.Run("names exclude exited by default", func(t *testing.T) {
		names, err := AutoCompleteListTerminalNames(ctx, nil, runPath, false)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !slices.Contains(names, "active") || slices.Contains(names, "exited") {
			t.Fatalf("names = %v, want active only", names)
		}
	})

	t.Run("names include exited when requested", func(t *testing.T) {
		names, err := AutoCompleteListTerminalNames(ctx, nil, runPath, true)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !slices.Contains(names, "active") || !slices.Contains(names, "exited") {
			t.Fatalf("names = %v, want both", names)
		}
	})

	t.Run("ids exclude exited by default", func(t *testing.T) {
		ids, err := AutoCompleteListTerminalIDs(ctx, nil, runPath, false)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !slices.Contains(ids, "id-active") || slices.Contains(ids, "id-exited") {
			t.Fatalf("ids = %v, want id-active only", ids)
		}
	})

	t.Run("ids include exited when requested", func(t *testing.T) {
		ids, err := AutoCompleteListTerminalIDs(ctx, nil, runPath, true)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("ids = %v, want both", ids)
		}
	})
}

func TestAutoCompleteListClientNames(t *testing.T) {
	ctx := context.Background()

	t.Run("empty run path yields no names", func(t *testing.T) {
		names, err := AutoCompleteListClientNames(ctx, nil, t.TempDir(), false)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(names) != 0 {
			t.Fatalf("names = %v, want empty", names)
		}
	})

	runPath := t.TempDir()
	writeClientDoc(t, runPath, "c-active", "active", api.ClientReady)
	writeClientDoc(t, runPath, "c-exited", "exited", api.ClientExited)

	t.Run("excludes exited by default", func(t *testing.T) {
		names, err := AutoCompleteListClientNames(ctx, nil, runPath, false)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !slices.Contains(names, "active") || slices.Contains(names, "exited") {
			t.Fatalf("names = %v, want active only", names)
		}
	})

	t.Run("includes exited when requested", func(t *testing.T) {
		names, err := AutoCompleteListClientNames(ctx, nil, runPath, true)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !slices.Contains(names, "active") || !slices.Contains(names, "exited") {
			t.Fatalf("names = %v, want both", names)
		}
	})
}
