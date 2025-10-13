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
func BuildSessionSpec(
	ctx context.Context,
	p *BuildSessionSpecParams,
) (*api.SessionSpec, error) {
	if p.SessionID == "" {
		// Default session ID to a random one
		p.SessionID = naming.RandomID()
	}

	if p.SessionName == "" {
		p.SessionName = naming.RandomName()
	}

	if p.ProfilesFile == "" {
		// Default profilesFilename to $RUN_PATH/profiles.yaml
		p.ProfilesFile = filepath.Join(p.RunPath, ".sbsh", "profiles.yaml")
	}

	if p.RunPath == "" {
		// Default runPath to $HOME/.sbsh
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		p.RunPath = filepath.Join(homeDir, ".sbsh", "run")
	}

	if p.SessionCmd == "" {
		p.SessionCmd = "/bin/bash"
		p.SessionCmdArgs = []string{"-i"}
	}

	if p.CaptureFile == "" {
		p.CaptureFile = filepath.Join(
			p.RunPath,
			"sessions",
			p.SessionID,
			"session.log",
		)
	}

	if p.SocketFile == "" {
		p.SocketFile = filepath.Join(
			p.RunPath,
			"sessions",
			p.SessionID,
			"socket",
		)
	}

	var sessionSpec *api.SessionSpec
	if p.ProfileName == "" {
		// No profile: build a SessionSpec from command-line inputs only.

		// Define a new Session
		sessionSpec = &api.SessionSpec{
			ID:          api.ID(p.SessionID),
			Kind:        api.SessionLocal,
			Name:        p.SessionName,
			Command:     p.SessionCmd,
			CommandArgs: p.SessionCmdArgs,
			Env:         os.Environ(),
			Prompt:      "(sbsh-$SBSH_SES_ID) $PS1",
			RunPath:     p.RunPath,
			CaptureFile: p.CaptureFile,
			LogFile:     p.LogFile,
			LogLevel:    p.LogLevel,
			SocketFile:  p.SocketFile,
		}
	} else {
		// Profile given: load profiles file, find profile by name, and build SessionSpec from it.
		profileSpec, err := discovery.FindProfileByName(ctx, p.ProfilesFile, p.ProfileName)
		if err != nil {
			return nil, err
		}
		sessionSpec, err = CreateSessionFromProfile(profileSpec)
		if err != nil {
			return nil, err
		}
		sessionSpec.ID = api.ID(p.SessionID)
		sessionSpec.Name = p.SessionName
		sessionSpec.RunPath = p.RunPath
		sessionSpec.CaptureFile = p.CaptureFile
		sessionSpec.LogFile = p.LogFile
		sessionSpec.LogLevel = p.LogLevel
		sessionSpec.Env = append(sessionSpec.Env, p.EnvVars...)
	}
	return sessionSpec, nil
}
