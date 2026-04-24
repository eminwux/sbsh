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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func DefaultRunPath() string {
	base, err := os.UserHomeDir()
	if err != nil {
		// fallback to tmp if home dir cannot be determined
		base = "/tmp/sbsh"
	}
	return filepath.Join(base, ".sbsh", "run")
}

func GetRunPathFromEnvAndFlags(cmd *cobra.Command, viperKey string) (string, error) {
	runPath, _ := cmd.Flags().GetString("run-path")
	if runPath == "" {
		if env := os.Getenv(viperKey); env != "" {
			runPath = env
		} else {
			// final fallback: same default you use at runtime
			runPath = DefaultRunPath()
		}
	}
	return runPath, nil
}

// GetProfilesDirFromEnvAndFlags resolves the profiles directory using the
// precedence flag > env > default. envVar is the environment variable to
// consult when the flag is unset.
func GetProfilesDirFromEnvAndFlags(cmd *cobra.Command, envVar string) (string, error) {
	profilesDir, _ := cmd.Flags().GetString("profiles-dir")
	if profilesDir == "" {
		if env := os.Getenv(envVar); env != "" {
			profilesDir = env
		} else {
			profilesDir = DefaultProfilesDir()
		}
	}
	return profilesDir, nil
}

// DefaultProfilesDir returns the default profiles directory
// ($HOME/.sbsh/profiles.d/).
func DefaultProfilesDir() string {
	base, err := os.UserHomeDir()
	if err != nil {
		// fallback to tmp if home dir cannot be determined
		base = "tmp"
	}

	return filepath.Join(base, ".sbsh", "profiles.d")
}

func DefaultConfigFile() string {
	base, err := os.UserHomeDir()
	if err != nil {
		// fallback to tmp if home dir cannot be determined
		base = "tmp"
	}

	return filepath.Join(base, ".sbsh", "config.yaml")
}
