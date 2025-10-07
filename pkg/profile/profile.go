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
	"os"
	"sort"

	"github.com/spf13/viper"
	"sbsh/pkg/api"
	"sbsh/pkg/discovery"
	"sbsh/pkg/env"
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
		Command:     profile.Spec.Shell.Cmd,
		CommandArgs: append([]string(nil), profile.Spec.Shell.CmdArgs...),
		Env:         envSlice,
		Labels:      copyStringMap(profile.Metadata.Labels),
		ProfileName: profile.Metadata.Name,
		Prompt:      profile.Spec.Shell.Prompt,
		// LogFilename, SockerCtrl, SocketIO, RunPath: left empty for caller/context to fill.
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

func BuildSessionSpec(
	profileNameInput, sessionIDInput, sessionNameInput, sessionCmdInput, logFilenameInput string,
	ctx context.Context,
) (*api.SessionSpec, error) {
	var sessionSpec *api.SessionSpec
	if profileNameInput == "" {
		// Split into args for exec
		cmdArgs := []string{}

		// Define a new Session
		sessionSpec = &api.SessionSpec{
			ID:          api.ID(sessionIDInput),
			Kind:        api.SessionLocal,
			Name:        sessionNameInput,
			Command:     sessionCmdInput,
			CommandArgs: cmdArgs,
			Env:         os.Environ(),
			RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
			LogFilename: logFilenameInput,
			// Prompt:      "(sbsh-$SBSH_SES_ID) $PS1",
			Prompt: "pepe>",
		}
	} else {
		profileSpec, err := discovery.FindProfileByName(ctx, viper.GetString(env.PROFILES_FILE.ViperKey), profileNameInput)
		if err != nil {
			return nil, err
		}
		sessionSpec, err = CreateSessionFromProfile(profileSpec)
		if err != nil {
			return nil, err
		}
		sessionSpec.ID = api.ID(sessionIDInput)
		sessionSpec.RunPath = viper.GetString(env.RUN_PATH.ViperKey)
		sessionSpec.LogFilename = logFilenameInput
		sessionSpec.Env = append(sessionSpec.Env, os.Environ()...)

		env.SES_PROFILE.Set(profileNameInput)

		err = env.SES_PROFILE.BindEnv()
		if err != nil {
			return nil, err
		}
	}
	return sessionSpec, nil
}
