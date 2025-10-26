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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/pkg/api"
)

// CreateSessionFromProfile converts a SessionProfileDoc (profile YAML) into a SessionSpec
// that sbsh can use to spawn a session. It maps only what's available in the profile
// schema today: name, runTarget -> kind, shell.{cmd,cmdArgs,env}. Other fields in
// SessionSpec (ID, LogFilename, Socket paths, RunPath) are left for the caller to fill.
func CreateSessionFromProfile(profile *api.SessionProfileDoc) (*api.SessionSpec, error) {
	if profile == nil {
		return nil, errors.New("profile is nil")
	}

	if profile.APIVersion == "" || profile.Kind == "" {
		return nil, errors.New("invalid profile: missing apiVersion/kind")
	}

	if profile.Kind != api.KindSessionProfile {
		return nil, fmt.Errorf("invalid kind %q (expected %q)", profile.Kind, api.KindSessionProfile)
	}

	if profile.Metadata.Name == "" {
		return nil, errors.New("invalid profile: metadata.name is required")
	}

	if profile.Spec.Shell.Cmd == "" {
		return nil, fmt.Errorf("invalid profile %q: shell.cmd is required", profile.Metadata.Name)
	}

	// Map runTarget -> SessionKind (limited to local for now).
	var kind api.SessionKind
	switch profile.Spec.RunTarget {
	case api.RunTargetLocal, "":
		kind = api.SessionLocal
	default:
		// For now, default unknown/unsupported targets to local so the caller can still run it,
		// or change this to return an error if you prefer strict behavior.
		kind = api.SessionLocal
	}

	// Map env (map[string]string) -> []string {"KEY=VAL"} with stable ordering.
	var envSlice []string
	if m := profile.Spec.Shell.Env; len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		envSlice = make([]string, 0, len(keys))
		for _, k := range keys {
			envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, m[k]))
		}
	}

	spec := &api.SessionSpec{
		// ID: zero; caller should set.
		Kind:        kind,
		Cwd:         profile.Spec.Shell.Cwd,
		Command:     profile.Spec.Shell.Cmd,
		CommandArgs: append([]string(nil), profile.Spec.Shell.CmdArgs...),
		EnvInherit:  profile.Spec.Shell.EnvInherit,
		Env:         envSlice,
		Labels:      copyStringMap(profile.Metadata.Labels),
		ProfileName: profile.Metadata.Name,
		Prompt:      profile.Spec.Shell.Prompt,
		Stages:      profile.Spec.Stages,
	}

	return spec, nil
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type BuildSessionSpecParams struct {
	SessionID      string
	SessionName    string
	SessionCmd     string
	SessionCmdArgs []string
	CaptureFile    string
	RunPath        string
	ProfilesFile   string
	ProfileName    string
	LogFile        string
	LogLevel       string
	SocketFile     string
	EnvVars        []string
}

// BuildSessionSpec builds a SessionSpec from command-line inputs and/or a profile.
// It applies defaults for missing values, and if a profile name is given, it loads
// the profiles file and merges the profile into the spec.
// The returned SessionSpec is ready to be used to spawn a session.
//
//nolint:funlen // For understandability, this function is long but straightforward.
func BuildSessionSpec(
	ctx context.Context,
	logger *slog.Logger,
	input *BuildSessionSpecParams,
) (*api.SessionSpec, error) {
	if input.SessionID == "" {
		// Default session ID to a random one
		input.SessionID = naming.RandomID()
	}

	if input.SessionName == "" {
		input.SessionName = naming.RandomName()
	}

	if input.ProfilesFile == "" {
		// Default profilesFilename to $RUN_PATH/profiles.yaml
		input.ProfilesFile = filepath.Join(input.RunPath, ".sbsh", "profiles.yaml")
	}

	if input.RunPath == "" {
		// Default runPath to $HOME/.sbsh
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		input.RunPath = filepath.Join(homeDir, ".sbsh", "run")
	}

	if input.SessionCmd == "" {
		input.SessionCmd = "/bin/bash"
		input.SessionCmdArgs = []string{"-i"}
	}

	if input.LogFile == "" {
		input.LogFile = filepath.Join(
			input.RunPath,
			"sessions",
			input.SessionID,
			"log",
		)
	}

	if input.LogLevel == "" {
		input.LogLevel = "info"
	}

	if input.CaptureFile == "" {
		input.CaptureFile = filepath.Join(
			input.RunPath,
			"sessions",
			input.SessionID,
			"capture",
		)
	}

	if input.SocketFile == "" {
		input.SocketFile = filepath.Join(
			input.RunPath,
			"sessions",
			input.SessionID,
			"socket",
		)
	}
	if input.ProfileName == "" {
		input.ProfileName = "default"
	}

	logger.DebugContext(
		ctx,
		"Profile specified, loading and applying profile",
		"profile",
		input.ProfileName,
		"profilesFile",
		input.ProfilesFile,
	)

	var sessionSpec *api.SessionSpec
	var profileSpec *api.SessionProfileDoc
	var errFind error

	profileSpec, errFind = discovery.FindProfileByName(ctx, logger, input.ProfilesFile, input.ProfileName)
	if errFind != nil {
		logger.InfoContext(
			ctx,
			"Profile not found",
			"profile",
			input.ProfileName,
		)

		if input.ProfileName != "default" {
			return nil, errFind
		}

		logger.InfoContext(
			ctx,
			"Default profile not found, reverting to hardcoded default profile",
			"profile",
			input.ProfileName,
		)

		profileSpec, errFind = GetDefaultHardcodedProfile(ctx, logger, input)
		if errFind != nil {
			logger.ErrorContext(ctx, "Failed to get default profile", "error", errFind)
			return nil, errFind
		}
	}

	var errCreate error
	sessionSpec, errCreate = CreateSessionFromProfile(profileSpec)
	if errCreate != nil {
		return nil, errCreate
	}
	addInputValuesToSession(sessionSpec, input)

	return sessionSpec, nil
}

// addInputValuesToSession mutates sessionSpec by overriding its fields with non-empty values from input.
// It sets ID, Name, RunPath, CaptureFile, LogFile, LogLevel, SocketFile, and appends EnvVars, avoiding duplicates.
func addInputValuesToSession(sessionSpec *api.SessionSpec, input *BuildSessionSpecParams) {
	sessionSpec.ID = api.ID(input.SessionID)
	sessionSpec.Name = input.SessionName
	sessionSpec.RunPath = input.RunPath
	sessionSpec.CaptureFile = input.CaptureFile
	sessionSpec.LogFile = input.LogFile
	sessionSpec.LogLevel = input.LogLevel
	sessionSpec.SocketFile = input.SocketFile
	sessionSpec.Env = append(sessionSpec.Env, input.EnvVars...)
}

// GetDefaultHardcodedProfile constructs a default SessionProfileDoc using command-line or session defaults.
// Used when no profile is found; logs the fallback and builds the profile from BuildSessionSpecParams.
func GetDefaultHardcodedProfile(
	ctx context.Context,
	logger *slog.Logger,
	input *BuildSessionSpecParams,
) (*api.SessionProfileDoc, error) {
	logger.DebugContext(ctx, "No profile specified, using command-line/session defaults")

	profileSpec := &api.SessionProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindSessionProfile,
		Metadata: api.SessionProfileMeta{
			Name:   "default",
			Labels: map[string]string{},
		},
		Spec: api.SessionProfileSpec{
			RunTarget: api.RunTargetLocal,
			Shell: api.ShellSpec{
				Cmd:        input.SessionCmd,
				CmdArgs:    input.SessionCmdArgs,
				EnvInherit: true,
				Env:        map[string]string{},
				Prompt:     "\"(sbsh-$SBSH_SES_ID) $PS1\"",
			},
			Stages: api.StagesSpec{},
		},
	}
	return profileSpec, nil
}
