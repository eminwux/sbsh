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
	"path/filepath"
	"sort"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/pkg/api"
)

// CreateTerminalFromProfile converts a TerminalProfileDoc (profile YAML) into a TerminalSpec
// that sbsh can use to spawn a terminal. It maps only what's available in the profile
// schema today: name, runTarget -> kind, shell.{cmd,cmdArgs,env}. Other fields in
// TerminalSpec (ID, LogFilename, Socket paths, RunPath) are left for the caller to fill.
func CreateTerminalFromProfile(profile *api.TerminalProfileDoc) (*api.TerminalSpec, error) {
	if profile == nil {
		return nil, errors.New("profile is nil")
	}

	if profile.APIVersion == "" || profile.Kind == "" {
		return nil, errors.New("invalid profile: missing apiVersion/kind")
	}

	if profile.Kind != api.KindTerminalProfile {
		return nil, fmt.Errorf("invalid kind %q (expected %q)", profile.Kind, api.KindTerminalProfile)
	}

	if profile.Metadata.Name == "" {
		return nil, errors.New("invalid profile: metadata.name is required")
	}

	if profile.Spec.Shell.Cmd == "" {
		return nil, fmt.Errorf("invalid profile %q: shell.cmd is required", profile.Metadata.Name)
	}

	// Map runTarget -> TerminalKind (limited to local for now).
	var kind api.TerminalKind
	switch profile.Spec.RunTarget {
	case api.RunTargetLocal, "":
		kind = api.TerminalLocal
	default:
		// For now, default unknown/unsupported targets to local so the caller can still run it,
		// or change this to return an error if you prefer strict behavior.
		kind = api.TerminalLocal
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

	spec := &api.TerminalSpec{
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
		SetPrompt:   true,
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

type BuildTerminalSpecParams struct {
	TerminalID       string
	TerminalName     string
	TerminalCmd      string
	TerminalCmdArgs  []string
	CaptureFile      string
	RunPath          string
	ProfilesFile     string
	ProfileName      string
	LogFile          string
	LogLevel         string
	SocketFile       string
	EnvVars          []string
	DisableSetPrompt bool
}

// BuildTerminalSpec builds a TerminalSpec from command-line inputs and/or a profile.
// It applies defaults for missing values, and if a profile name is given, it loads
// the profiles file and merges the profile into the spec.
// The returned TerminalSpec is ready to be used to spawn a terminal.
//
//nolint:funlen // For understandability, this function is long but straightforward.
func BuildTerminalSpec(
	ctx context.Context,
	logger *slog.Logger,
	input *BuildTerminalSpecParams,
) (*api.TerminalSpec, error) {
	if input.TerminalID == "" {
		// Default terminal ID to a random one
		input.TerminalID = naming.RandomID()
	}

	if input.TerminalName == "" {
		input.TerminalName = naming.RandomName()
	}

	if input.ProfilesFile == "" {
		// Default profilesFilename to $RUN_PATH/profiles.yaml
		input.ProfilesFile = filepath.Join(input.RunPath, ".sbsh", "profiles.yaml")
	}

	if input.RunPath == "" {
		// Default runPath to $HOME/.sbsh
		input.RunPath = config.DefaultRunPath()
	}

	if input.TerminalCmd == "" {
		input.TerminalCmd = "/bin/bash"
		input.TerminalCmdArgs = []string{"-i"}
	}

	if input.LogFile == "" {
		input.LogFile = filepath.Join(
			input.RunPath,
			defaults.TerminalsRunPath,
			input.TerminalID,
			"log",
		)
	}

	if input.LogLevel == "" {
		input.LogLevel = "info"
	}

	if input.CaptureFile == "" {
		input.CaptureFile = filepath.Join(
			input.RunPath,
			defaults.TerminalsRunPath,
			input.TerminalID,
			"capture",
		)
	}

	if input.SocketFile == "" {
		input.SocketFile = filepath.Join(
			input.RunPath,
			defaults.TerminalsRunPath,
			input.TerminalID,
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

	var terminalSpec *api.TerminalSpec
	var profileSpec *api.TerminalProfileDoc
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
	terminalSpec, errCreate = CreateTerminalFromProfile(profileSpec)
	if errCreate != nil {
		return nil, errCreate
	}
	addInputValuesToTerminal(terminalSpec, input)

	return terminalSpec, nil
}

// addInputValuesToTerminal mutates terminalSpec by overriding its fields with non-empty values from input.
// It sets ID, Name, RunPath, CaptureFile, LogFile, LogLevel, SocketFile, and appends EnvVars, avoiding duplicates.
func addInputValuesToTerminal(terminalSpec *api.TerminalSpec, input *BuildTerminalSpecParams) {
	terminalSpec.ID = api.ID(input.TerminalID)
	terminalSpec.Name = input.TerminalName
	terminalSpec.RunPath = input.RunPath
	terminalSpec.CaptureFile = input.CaptureFile
	terminalSpec.LogFile = input.LogFile
	terminalSpec.LogLevel = input.LogLevel
	terminalSpec.SocketFile = input.SocketFile
	terminalSpec.Env = append(terminalSpec.Env, input.EnvVars...)
	// Inverted logic: when DisableSetPrompt is true, SetPrompt is false
	terminalSpec.SetPrompt = !input.DisableSetPrompt
}

// GetDefaultHardcodedProfile constructs a default TerminalProfileDoc using command-line or terminal defaults.
// Used when no profile is found; logs the fallback and builds the profile from BuildTerminalSpecParams.
func GetDefaultHardcodedProfile(
	ctx context.Context,
	logger *slog.Logger,
	input *BuildTerminalSpecParams,
) (*api.TerminalProfileDoc, error) {
	logger.DebugContext(ctx, "No profile specified, using command-line/terminal defaults")

	profileSpec := &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata: api.TerminalProfileMetadata{
			Name:   "default",
			Labels: map[string]string{},
		},
		Spec: api.TerminalProfileSpec{
			RunTarget: api.RunTargetLocal,
			Shell: api.ShellSpec{
				Cmd:        input.TerminalCmd,
				CmdArgs:    input.TerminalCmdArgs,
				EnvInherit: true,
				Env:        map[string]string{},
				Prompt:     "\"(sbsh-$SBSH_TERM_ID) $PS1\"",
			},
			Stages: api.StagesSpec{},
		},
	}
	return profileSpec, nil
}
