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

package builder

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/pkg/api"
)

// TerminalOption mutates a terminalConfig. Later options override
// earlier ones so callers can compose defaults + overrides.
type TerminalOption func(*terminalConfig)

// terminalConfig is the internal accumulator for TerminalOption
// values. It mirrors the subset of profile.BuildTerminalSpecParams
// that external callers are expected to control.
type terminalConfig struct {
	id               string
	name             string
	command          string
	commandArgs      []string
	envVars          []string
	cwd              string
	captureFile      string
	logFile          string
	logLevel         string
	socketFile       string
	profileName      string
	profilesDir      string
	disableSetPrompt bool
}

// WithID sets the terminal ID. Empty means "generate a random ID".
func WithID(id string) TerminalOption {
	return func(c *terminalConfig) { c.id = id }
}

// WithName sets the human-readable terminal name. Empty means
// "generate a random name".
func WithName(name string) TerminalOption {
	return func(c *terminalConfig) { c.name = name }
}

// WithProfile selects a profile by Metadata.Name from the directory
// resolved via WithProfilesDir (or the default location if no
// explicit path is set). Empty resolves to the hardcoded "default"
// profile.
func WithProfile(name string) TerminalOption {
	return func(c *terminalConfig) { c.profileName = name }
}

// WithProfilesDir points the builder at a directory that is scanned
// recursively for *.yaml / *.yml files containing TerminalProfile
// documents. Empty means "use the default profiles directory
// ($HOME/.sbsh/profiles.d/)".
func WithProfilesDir(path string) TerminalOption {
	return func(c *terminalConfig) { c.profilesDir = path }
}

// WithCommand sets the terminal's command and its argv. argv[0] is
// the command binary; argv[1:] are its arguments. An empty or nil
// argv leaves the value untouched (profile/default wins).
func WithCommand(argv []string) TerminalOption {
	return func(c *terminalConfig) {
		if len(argv) == 0 || argv[0] == "" {
			return
		}
		c.command = argv[0]
		c.commandArgs = append([]string(nil), argv[1:]...)
	}
}

// WithCwd sets the working directory for the spawned shell. Empty
// means "inherit from the profile, or default to the parent's cwd".
func WithCwd(path string) TerminalOption {
	return func(c *terminalConfig) { c.cwd = path }
}

// WithEnv adds environment variables to the spawned shell. Keys
// are applied in a stable (lexicographic) order so repeated calls
// produce deterministic results. The resulting entries are
// appended to whatever the profile defines — they do not replace
// the profile's env map.
func WithEnv(env map[string]string) TerminalOption {
	return func(c *terminalConfig) {
		if len(env) == 0 {
			return
		}
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c.envVars = append(c.envVars, fmt.Sprintf("%s=%s", k, env[k]))
		}
	}
}

// WithCaptureFile overrides the on-disk capture file path. Empty
// means "derive from runPath + terminal ID".
func WithCaptureFile(path string) TerminalOption {
	return func(c *terminalConfig) { c.captureFile = path }
}

// WithLogFile overrides the on-disk log file path. Empty means
// "derive from runPath + terminal ID".
func WithLogFile(path string) TerminalOption {
	return func(c *terminalConfig) { c.logFile = path }
}

// WithLogLevel sets the terminal's log verbosity (e.g. "info",
// "debug"). Empty means "use the default".
func WithLogLevel(level string) TerminalOption {
	return func(c *terminalConfig) { c.logLevel = level }
}

// WithSocketFile overrides the terminal's control-socket path.
// Empty means "derive from runPath + terminal ID".
func WithSocketFile(path string) TerminalOption {
	return func(c *terminalConfig) { c.socketFile = path }
}

// WithDisableSetPrompt disables sbsh's automatic PS1 injection
// when true. The default (false) preserves the shell-prompt
// rewriting behavior.
func WithDisableSetPrompt(disable bool) TerminalOption {
	return func(c *terminalConfig) { c.disableSetPrompt = disable }
}

// BuildTerminalSpec produces a TerminalSpec from a runPath plus an
// optional profile selection and inline overrides. It wraps
// internal/profile.BuildTerminalSpec and keeps the same resolution
// rules: inline option values override profile values, and a
// missing profile falls back to the hardcoded "default" profile
// only when the requested profile name is literally "default" (or
// empty).
//
// runPath is required; an empty runPath returns
// errdefs.ErrRunPathRequired.
func BuildTerminalSpec(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	opts ...TerminalOption,
) (*api.TerminalSpec, error) {
	if runPath == "" {
		return nil, errdefs.ErrRunPathRequired
	}

	cfg := terminalConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return profile.BuildTerminalSpec(ctx, logger, &profile.BuildTerminalSpecParams{
		TerminalID:       cfg.id,
		TerminalName:     cfg.name,
		TerminalCmd:      cfg.command,
		TerminalCmdArgs:  cfg.commandArgs,
		Cwd:              cfg.cwd,
		CaptureFile:      cfg.captureFile,
		RunPath:          runPath,
		ProfilesDir:      cfg.profilesDir,
		ProfileName:      cfg.profileName,
		LogFile:          cfg.logFile,
		LogLevel:         cfg.logLevel,
		SocketFile:       cfg.socketFile,
		EnvVars:          cfg.envVars,
		DisableSetPrompt: cfg.disableSetPrompt,
	})
}
