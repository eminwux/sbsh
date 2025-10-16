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

func DefaultRunPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sbsh", "run"), nil
}

func GetRunPathFromEnvAndFlags(cmd *cobra.Command) (string, error) {
	runPath, _ := cmd.Flags().GetString("run-path")
	if runPath == "" {
		if env := os.Getenv(RUN_PATH.EnvVar()); env != "" {
			runPath = env
		} else {
			// final fallback: same default you use at runtime
			var err error
			runPath, err = DefaultRunPath()
			if err != nil {
				return "", err
			}
		}
	}
	return runPath, nil
}

func GetProfilesFileFromEnvAndFlags(cmd *cobra.Command) (string, error) {
	profilesFile, _ := cmd.Flags().GetString("profiles-file")
	if profilesFile == "" {
		if env := os.Getenv(PROFILES_FILE.EnvVar()); env != "" {
			profilesFile = env
		} else {
			// final fallback: same default you use at runtime
			var err error
			profilesFile, err = DefaultProfilesFile()
			if err != nil {
				return "", err
			}
		}
	}
	return profilesFile, nil
}

func DefaultProfilesFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sbsh", "profiles.yaml"), nil
}
