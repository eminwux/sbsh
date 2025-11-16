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
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Test_setupRootCmd_HappyPath(t *testing.T) {
	t.Cleanup(func() {
		viper.Reset()
	})

	rootCmd := &cobra.Command{Use: "sb"}
	if err := setupRootCmd(rootCmd); err != nil {
		t.Fatalf("setupRootCmd() error = %v", err)
	}

	flagCases := []struct {
		name     string
		flagName string
		value    string
		viperKey string
		isBool   bool
	}{
		{
			name:     "config",
			flagName: "config",
			value:    "/tmp/config.yaml",
			viperKey: config.SB_ROOT_CONFIG.ViperKey,
		},
		{
			name:     "verbose",
			flagName: "verbose",
			value:    "true",
			viperKey: config.SB_ROOT_VERBOSE.ViperKey,
			isBool:   true,
		},
		{
			name:     "log-level",
			flagName: "log-level",
			value:    "debug",
			viperKey: config.SB_ROOT_LOG_LEVEL.ViperKey,
		},
		{
			name:     "run-path",
			flagName: "run-path",
			value:    "/tmp/sb/run",
			viperKey: config.SB_ROOT_RUN_PATH.ViperKey,
		},
	}

	for _, tc := range flagCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := rootCmd.PersistentFlags().Set(tc.flagName, tc.value); err != nil {
				t.Fatalf("failed to set flag %s: %v", tc.flagName, err)
			}
			if tc.isBool {
				if got := viper.GetBool(tc.viperKey); !got {
					t.Fatalf("viper key %s expected to be true", tc.viperKey)
				}
				return
			}
			if got := viper.GetString(tc.viperKey); got != tc.value {
				t.Fatalf("viper key %s expected %s, got %s", tc.viperKey, tc.value, got)
			}
		})
	}
}

func Test_LoadConfig_HappyPath(t *testing.T) {
	t.Cleanup(func() {
		viper.Reset()
	})

	t.Setenv(config.SBSH_ROOT_CONFIG_FILE.EnvVar(), "")
	t.Setenv(config.SB_ROOT_RUN_PATH.EnvVar(), "")
	t.Setenv(config.SB_GET_PROFILES_FILE.EnvVar(), "")
	t.Setenv(config.SBSH_ROOT_LOG_LEVEL.EnvVar(), "")

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if got, want := viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey), config.DefaultRunPath(); got != want {
		t.Fatalf("expected run path %s, got %s", want, got)
	}

	if got, want := viper.GetString(config.SB_GET_PROFILES_FILE.ViperKey), config.DefaultProfilesFile(); got != want {
		t.Fatalf("expected get profiles file %s, got %s", want, got)
	}

	if got := viper.GetString(config.SBSH_ROOT_LOG_LEVEL.ViperKey); got != "info" {
		t.Fatalf("expected log level info, got %s", got)
	}
}
