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
	id                string
	name              string
	command           string
	commandArgs       []string
	envVars           []string
	cwd               string
	captureFile       string
	captureMode       string
	captureGID        *int
	logFile           string
	logFileMode       string
	logFileGID        *int
	logLevel          string
	socketFile        string
	socketMode        string
	socketGID         *int
	profileName       string
	profilesDir       string
	profileSet        bool
	profilesDirSet    bool
	disableSetPrompt  bool
	stages            api.StagesSpec
	stagesOverlay     bool
	onInitOverlay     bool
	postAttachOverlay bool
	prompt            string
	promptSet         bool
	envInherit        bool
	envInheritSet     bool
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
//
// Honored only by BuildTerminalSpecFromProfile. Passing this option to
// the inline BuildTerminalSpec returns an errdefs.ErrInvalidOption.
func WithProfile(name string) TerminalOption {
	return func(c *terminalConfig) {
		c.profileName = name
		c.profileSet = true
	}
}

// WithProfilesDir points the builder at a directory that is scanned
// recursively for *.yaml / *.yml files containing TerminalProfile
// documents. Empty means "use the default profiles directory
// ($HOME/.sbsh/profiles.d/)".
//
// Honored only by BuildTerminalSpecFromProfile. Passing this option to
// the inline BuildTerminalSpec returns an errdefs.ErrInvalidOption.
func WithProfilesDir(path string) TerminalOption {
	return func(c *terminalConfig) {
		c.profilesDir = path
		c.profilesDirSet = true
	}
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

// WithSocketMode sets the chmod mode applied to the control
// socket after Listen, as an octal string ("0660", "660"). Empty
// means "no override; fall through to the profile or the runner
// default (0o600)".
func WithSocketMode(mode string) TerminalOption {
	return func(c *terminalConfig) { c.socketMode = mode }
}

// WithSocketGID sets the numeric GID applied via chown to the
// control socket after Listen. Calling this option means
// "explicitly set"; the zero value is a valid GID (root) and is
// preserved. Omitting the option leaves the group unchanged (or
// falls through to the profile).
func WithSocketGID(gid int) TerminalOption {
	return func(c *terminalConfig) {
		g := gid
		c.socketGID = &g
	}
}

// WithCaptureMode sets the chmod mode applied to the capture file
// after Open, as an octal string ("0640", "640"). Empty means "no
// override; fall through to the profile or the runner default
// (0o600)".
func WithCaptureMode(mode string) TerminalOption {
	return func(c *terminalConfig) { c.captureMode = mode }
}

// WithCaptureGID sets the numeric GID applied via chown to the
// capture file after Open. Calling this option means "explicitly
// set"; the zero value is a valid GID (root) and is preserved.
// Omitting the option leaves the group unchanged (or falls through
// to the profile).
func WithCaptureGID(gid int) TerminalOption {
	return func(c *terminalConfig) {
		g := gid
		c.captureGID = &g
	}
}

// WithLogFileMode sets the chmod mode applied to the log file
// after Open, as an octal string ("0640", "640"). Empty means "no
// override; fall through to the profile or the runner default
// (0o600)".
func WithLogFileMode(mode string) TerminalOption {
	return func(c *terminalConfig) { c.logFileMode = mode }
}

// WithLogFileGID sets the numeric GID applied via chown to the log
// file after Open. Calling this option means "explicitly set"; the
// zero value is a valid GID (root) and is preserved. Omitting the
// option leaves the group unchanged (or falls through to the
// profile).
func WithLogFileGID(gid int) TerminalOption {
	return func(c *terminalConfig) {
		g := gid
		c.logFileGID = &g
	}
}

// WithDisableSetPrompt disables sbsh's automatic PS1 injection
// when true. The default (false) preserves the shell-prompt
// rewriting behavior.
func WithDisableSetPrompt(disable bool) TerminalOption {
	return func(c *terminalConfig) { c.disableSetPrompt = disable }
}

// WithStages overlays the full StagesSpec onto the resulting
// TerminalSpec.Stages. Takes precedence over WithOnInit /
// WithPostAttach when called: a later WithStages replaces both
// sub-fields with whatever the StagesSpec carries (including empty
// slices).
func WithStages(s api.StagesSpec) TerminalOption {
	return func(c *terminalConfig) {
		c.stages = s
		c.stagesOverlay = true
		c.onInitOverlay = false
		c.postAttachOverlay = false
	}
}

// WithOnInit overlays only Spec.Stages.OnInit. Compose with
// WithPostAttach to populate both sub-fields independently. If a
// later WithStages call follows, it replaces everything WithOnInit
// set.
func WithOnInit(steps []api.ExecStep) TerminalOption {
	return func(c *terminalConfig) {
		c.stages.OnInit = append([]api.ExecStep(nil), steps...)
		c.onInitOverlay = true
		// A bare WithOnInit must not promote to a full-stages overlay,
		// otherwise PostAttach would silently be cleared on the inline
		// lane.
		c.stagesOverlay = false
	}
}

// WithPostAttach overlays only Spec.Stages.PostAttach. Mirror of
// WithOnInit.
func WithPostAttach(steps []api.ExecStep) TerminalOption {
	return func(c *terminalConfig) {
		c.stages.PostAttach = append([]api.ExecStep(nil), steps...)
		c.postAttachOverlay = true
		c.stagesOverlay = false
	}
}

// WithPrompt stamps Spec.Prompt with the supplied value. Empty is a
// valid value — callers that want sbsh's runner to skip the prompt
// rewrite should pair this with WithDisableSetPrompt(true) instead
// of relying on emptiness.
func WithPrompt(prompt string) TerminalOption {
	return func(c *terminalConfig) {
		c.prompt = prompt
		c.promptSet = true
	}
}

// WithEnvInherit stamps Spec.EnvInherit. The set-sentinel discipline
// is required because false is a meaningful value distinct from
// "caller did not say".
func WithEnvInherit(b bool) TerminalOption {
	return func(c *terminalConfig) {
		c.envInherit = b
		c.envInheritSet = true
	}
}

// BuildTerminalSpec produces a TerminalSpec from a runPath plus
// inline TerminalOption values. It does not load a YAML profile, does
// not run pkg/discovery, and does not fall back to the hardcoded
// "default" profile — every shell-shaped field on the resulting spec
// comes from the With* options the caller passed (modulo the
// Cmd/CmdArgs default of /bin/bash -i so the runner has a sensible
// shell when the caller does not set one).
//
// Passing WithProfile or WithProfilesDir is rejected with
// errdefs.ErrInvalidOption — those options are only honored by
// BuildTerminalSpecFromProfile. Mixing the two lanes silently would
// hide a programmer bug; fail loud instead.
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
	if cfg.profileSet {
		return nil, fmt.Errorf(
			"%w: WithProfile is only honored by BuildTerminalSpecFromProfile",
			errdefs.ErrInvalidOption,
		)
	}
	if cfg.profilesDirSet {
		return nil, fmt.Errorf(
			"%w: WithProfilesDir is only honored by BuildTerminalSpecFromProfile",
			errdefs.ErrInvalidOption,
		)
	}

	return profile.BuildTerminalSpecInline(ctx, logger, paramsFromConfig(&cfg, runPath))
}

// BuildTerminalSpecFromProfile produces a TerminalSpec by loading a
// YAML profile from pkg/discovery and overlaying inline With* values
// on top. It preserves the legacy resolution rules: an empty
// WithProfile resolves to "default", a missing "default" falls back
// to the hardcoded profile, and inline options override profile-derived
// values for the matching fields.
//
// Use this entry point when the caller wants profile-driven defaults
// (the in-tree CLI lane, profile-driven tests). SDK consumers that
// want to build a spec entirely from in-memory fields should call
// BuildTerminalSpec instead.
//
// runPath is required; an empty runPath returns
// errdefs.ErrRunPathRequired.
func BuildTerminalSpecFromProfile(
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

	return profile.BuildTerminalSpecFromProfile(ctx, logger, paramsFromConfig(&cfg, runPath))
}

// paramsFromConfig is the single conversion point between the
// builder's option accumulator and the internal/profile parameter
// struct. Keeping it in one place ensures the two lanes share the
// exact same set of inline fields — adding a new With* option means
// editing exactly one mapping.
func paramsFromConfig(cfg *terminalConfig, runPath string) *profile.BuildTerminalSpecParams {
	return &profile.BuildTerminalSpecParams{
		TerminalID:        cfg.id,
		TerminalName:      cfg.name,
		TerminalCmd:       cfg.command,
		TerminalCmdArgs:   cfg.commandArgs,
		Cwd:               cfg.cwd,
		CaptureFile:       cfg.captureFile,
		RunPath:           runPath,
		ProfilesDir:       cfg.profilesDir,
		ProfileName:       cfg.profileName,
		LogFile:           cfg.logFile,
		LogLevel:          cfg.logLevel,
		SocketFile:        cfg.socketFile,
		SocketMode:        cfg.socketMode,
		SocketGID:         cfg.socketGID,
		CaptureMode:       cfg.captureMode,
		CaptureGID:        cfg.captureGID,
		LogFileMode:       cfg.logFileMode,
		LogFileGID:        cfg.logFileGID,
		EnvVars:           cfg.envVars,
		DisableSetPrompt:  cfg.disableSetPrompt,
		Stages:            cfg.stages,
		StagesOverlay:     cfg.stagesOverlay,
		OnInitOverlay:     cfg.onInitOverlay,
		PostAttachOverlay: cfg.postAttachOverlay,
		Prompt:            cfg.prompt,
		PromptSet:         cfg.promptSet,
		EnvInherit:        cfg.envInherit,
		EnvInheritSet:     cfg.envInheritSet,
	}
}
