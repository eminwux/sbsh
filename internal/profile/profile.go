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
	"strconv"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
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

	if errPerm := applyProfilePermBlock(
		profile.Spec.Socket, profile.Metadata.Name, "spec.socket",
		&spec.SocketMode, &spec.SocketGID,
	); errPerm != nil {
		return nil, errPerm
	}
	if errPerm := applyProfilePermBlock(
		profile.Spec.Capture, profile.Metadata.Name, "spec.capture",
		&spec.CaptureMode, &spec.CaptureGID,
	); errPerm != nil {
		return nil, errPerm
	}
	if errPerm := applyProfilePermBlock(
		profile.Spec.LogFile, profile.Metadata.Name, "spec.logFile",
		&spec.LogFileMode, &spec.LogFileGID,
	); errPerm != nil {
		return nil, errPerm
	}

	return spec, nil
}

// applyProfilePermBlock validates and copies a profile's permissions block
// (socket / capture / logFile — all share api.FilePermSpec) into the
// matching TerminalSpec fields. fieldPath ("spec.socket" etc.) is folded
// into error messages so callers can locate the offending YAML field.
func applyProfilePermBlock(
	block *api.FilePermSpec,
	profileName, fieldPath string,
	mode *os.FileMode,
	gid **int,
) error {
	if block == nil {
		return nil
	}
	if block.Mode != "" {
		m, err := parseFileMode(block.Mode)
		if err != nil {
			return fmt.Errorf("invalid %s.mode in profile %q: %w", fieldPath, profileName, err)
		}
		*mode = m
	}
	if block.GID != nil {
		g := *block.GID
		if g < 0 {
			return fmt.Errorf(
				"invalid %s.gid in profile %q: must be non-negative, got %d",
				fieldPath, profileName, g,
			)
		}
		*gid = &g
	}
	return nil
}

// chmodMask is the standard chmod surface: setuid/setgid/sticky plus the nine
// rwx bits. parseFileMode rejects any mode with bits outside this mask.
const chmodMask uint64 = 0o7777

// parseFileMode parses an octal mode string like "0660" or "660" into an
// os.FileMode and rejects modes with bits outside chmodMask. The leading "0"
// is optional because strconv.ParseUint with base 8 accepts both forms.
// Used for the control socket, capture file, and log file permission blocks
// — all three share the same syntax surface.
func parseFileMode(s string) (os.FileMode, error) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("not an octal mode: %w", err)
	}
	if n & ^chmodMask != 0 {
		return 0, fmt.Errorf("mode bits outside 0o%o: 0o%o", chmodMask, n)
	}
	return os.FileMode(n), nil
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
	TerminalID      string
	TerminalName    string
	TerminalCmd     string
	TerminalCmdArgs []string
	Cwd             string
	CaptureFile     string
	RunPath         string
	ProfilesDir     string
	ProfileName     string
	LogFile         string
	LogLevel        string
	SocketFile      string
	// SocketMode is the raw octal flag value (e.g. "0660"). Empty means
	// "no override; fall through to the profile or the runner default".
	SocketMode string
	// SocketGID is the numeric GID flag value, or nil to fall through to
	// the profile / leave the group unchanged. A nil pointer is required
	// because a zero int is a valid GID (root), so we can't conflate it
	// with "unset".
	SocketGID *int
	// CaptureMode is the raw octal flag value applied to the capture
	// file. Empty means "no override; fall through to the profile or the
	// runner default".
	CaptureMode string
	// CaptureGID is the numeric GID flag value for the capture file, or
	// nil to fall through to the profile / leave the group unchanged.
	CaptureGID *int
	// LogFileMode is the raw octal flag value applied to the log file.
	// Empty means "no override; fall through to the profile or the runner
	// default".
	LogFileMode string
	// LogFileGID is the numeric GID flag value for the log file, or nil
	// to fall through to the profile / leave the group unchanged.
	LogFileGID       *int
	EnvVars          []string
	DisableSetPrompt bool
	ShutdownGrace    time.Duration
	// Stages carries the inline-overlay values for Spec.Stages. The
	// *Overlay sentinels below decide which sub-fields are applied; an
	// unset overlay leaves the profile-derived value untouched. For the
	// inline lane the spec starts from zero, so the overlay flags decide
	// whether the field is populated at all.
	Stages api.StagesSpec
	// StagesOverlay overlays the full Stages value; takes precedence over
	// OnInitOverlay / PostAttachOverlay so a caller that mixes them gets
	// the full-replacement semantics they asked for.
	StagesOverlay bool
	// OnInitOverlay overlays only Stages.OnInit when StagesOverlay is
	// false.
	OnInitOverlay bool
	// PostAttachOverlay overlays only Stages.PostAttach when
	// StagesOverlay is false.
	PostAttachOverlay bool
	// Prompt + PromptSet: PromptSet=true means "stamp Prompt onto the
	// spec"; false leaves the profile-derived (or zero) value alone.
	Prompt    string
	PromptSet bool
	// EnvInherit + EnvInheritSet follow the same set-sentinel discipline
	// — false is a valid value, so we need an explicit "did the caller
	// set this" bit.
	EnvInherit    bool
	EnvInheritSet bool
}

// applyParamDefaults resolves the per-terminal defaults that both
// BuildTerminalSpecFromProfile and BuildTerminalSpecInline need: random
// ID / name, runPath-derived paths for log/capture/socket, the inline
// command default, and the profiles-dir fallback. Mutates input in place
// the same way the legacy single-function path did, so caller-visible
// fields the CLI later round-trips into TerminalStatus stay populated.
//
// RunPath is the only required input; callers must pre-check it before
// invoking this helper.
func applyParamDefaults(input *BuildTerminalSpecParams) {
	if input.TerminalID == "" {
		input.TerminalID = naming.RandomID()
	}
	if input.TerminalName == "" {
		input.TerminalName = naming.RandomName()
	}
	if input.ProfilesDir == "" {
		// Fallback: sit next to run state under $RUN_PATH so profile lookup
		// still has a well-defined path when the caller has not resolved the
		// $HOME-based default (which normally happens in LoadConfig).
		input.ProfilesDir = filepath.Join(input.RunPath, ".sbsh", "profiles.d")
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
}

// BuildTerminalSpecFromProfile builds a TerminalSpec by loading a YAML
// profile from pkg/discovery and overlaying the caller-provided inline
// values. The legacy behavior of this package: empty ProfileName resolves
// to "default", and a missing "default" falls back to the hardcoded
// profile from GetDefaultHardcodedProfile.
//
// RunPath is required; an empty RunPath returns errdefs.ErrRunPathRequired.
//
// SDK callers that want a profile-free path (no disk lookup, no implicit
// "default" resolution) should call BuildTerminalSpecInline instead.
//

func BuildTerminalSpecFromProfile(
	ctx context.Context,
	logger *slog.Logger,
	input *BuildTerminalSpecParams,
) (*api.TerminalSpec, error) {
	if input.RunPath == "" {
		return nil, errdefs.ErrRunPathRequired
	}

	applyParamDefaults(input)

	if input.ProfileName == "" {
		input.ProfileName = "default"
	}

	logger.DebugContext(
		ctx,
		"Profile specified, loading and applying profile",
		"profile",
		input.ProfileName,
		"profilesDir",
		input.ProfilesDir,
	)

	var terminalSpec *api.TerminalSpec

	profileSpec, warnings, errFind := discovery.FindProfileByNameInDir(
		ctx,
		logger,
		input.ProfilesDir,
		input.ProfileName,
	)
	for _, w := range warnings {
		logger.WarnContext(ctx, "profile loader warning", "file", w.File, "doc", w.DocIndex, "reason", w.Reason)
	}
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
	if errOv := applyPermOverrides(terminalSpec, input); errOv != nil {
		return nil, errOv
	}

	return terminalSpec, nil
}

// BuildTerminalSpecInline builds a TerminalSpec from inline fields only.
// It does not touch pkg/discovery, does not resolve an implicit "default"
// profile, and does not invoke GetDefaultHardcodedProfile. The returned
// spec has zero-valued Stages / Prompt / EnvInherit unless the caller's
// *Overlay / *Set sentinels are true, modulo the Cmd / CmdArgs defaults
// that applyParamDefaults applies so the runner has a sensible shell.
//
// RunPath is required; an empty RunPath returns errdefs.ErrRunPathRequired.
//
// Misuse — passing a ProfileName or ProfilesDir — must be rejected by the
// caller (pkg/builder enforces this at the public boundary).
func BuildTerminalSpecInline(
	ctx context.Context,
	logger *slog.Logger,
	input *BuildTerminalSpecParams,
) (*api.TerminalSpec, error) {
	if input.RunPath == "" {
		return nil, errdefs.ErrRunPathRequired
	}

	applyParamDefaults(input)

	logger.DebugContext(
		ctx,
		"Building terminal spec from inline options (no profile lookup)",
		"terminalID", input.TerminalID,
	)

	terminalSpec := &api.TerminalSpec{
		Kind:        api.TerminalLocal,
		Command:     input.TerminalCmd,
		CommandArgs: append([]string(nil), input.TerminalCmdArgs...),
	}
	addInputValuesToTerminal(terminalSpec, input)
	if errOv := applyPermOverrides(terminalSpec, input); errOv != nil {
		return nil, errOv
	}

	return terminalSpec, nil
}

// applyPermOverrides resolves the mode/gid precedence for the socket,
// capture, and log files: explicit flag/env values win over the profile's
// matching spec block; unset stays at whatever CreateTerminalFromProfile
// produced (which is the profile value or the zero value, the latter
// telling the runner to use 0o600 with no chown).
func applyPermOverrides(spec *api.TerminalSpec, input *BuildTerminalSpecParams) error {
	if err := applyOnePermOverride(
		input.SocketMode, input.SocketGID, "socket",
		&spec.SocketMode, &spec.SocketGID,
	); err != nil {
		return err
	}
	if err := applyOnePermOverride(
		input.CaptureMode, input.CaptureGID, "capture",
		&spec.CaptureMode, &spec.CaptureGID,
	); err != nil {
		return err
	}
	if err := applyOnePermOverride(
		input.LogFileMode, input.LogFileGID, "log-file",
		&spec.LogFileMode, &spec.LogFileGID,
	); err != nil {
		return err
	}
	return nil
}

// applyOnePermOverride applies an explicit flag/env override (rawMode +
// gidPtr) onto the matching TerminalSpec fields. label is folded into
// error messages so callers see which artifact rejected the input.
func applyOnePermOverride(
	rawMode string,
	gidPtr *int,
	label string,
	mode *os.FileMode,
	gid **int,
) error {
	if rawMode != "" {
		m, err := parseFileMode(rawMode)
		if err != nil {
			return fmt.Errorf("%w: invalid %s mode %q: %w", errdefs.ErrInvalidFlag, label, rawMode, err)
		}
		*mode = m
	}
	if gidPtr != nil {
		g := *gidPtr
		*gid = &g
	}
	return nil
}

// addInputValuesToTerminal mutates terminalSpec by overriding its fields with non-empty values from input.
// It sets ID, Name, RunPath, CaptureFile, LogFile, LogLevel, SocketFile, and appends EnvVars, avoiding duplicates.
// Cwd overrides the profile's Shell.Cwd only when non-empty so profile values stay sticky by default.
// Stages / Prompt / EnvInherit overlays follow the *Overlay / *Set sentinels — false leaves whatever
// the profile path (or zero-init) produced untouched.
func addInputValuesToTerminal(terminalSpec *api.TerminalSpec, input *BuildTerminalSpecParams) {
	terminalSpec.ID = api.ID(input.TerminalID)
	terminalSpec.Name = input.TerminalName
	terminalSpec.RunPath = input.RunPath
	terminalSpec.CaptureFile = input.CaptureFile
	terminalSpec.LogFile = input.LogFile
	terminalSpec.LogLevel = input.LogLevel
	terminalSpec.SocketFile = input.SocketFile
	if input.Cwd != "" {
		terminalSpec.Cwd = input.Cwd
	}
	terminalSpec.Env = append(terminalSpec.Env, input.EnvVars...)
	// Inverted logic: when DisableSetPrompt is true, SetPrompt is false
	terminalSpec.SetPrompt = !input.DisableSetPrompt
	if input.ShutdownGrace > 0 {
		terminalSpec.ShutdownGrace = input.ShutdownGrace
	}
	switch {
	case input.StagesOverlay:
		terminalSpec.Stages = input.Stages
	default:
		if input.OnInitOverlay {
			terminalSpec.Stages.OnInit = input.Stages.OnInit
		}
		if input.PostAttachOverlay {
			terminalSpec.Stages.PostAttach = input.Stages.PostAttach
		}
	}
	if input.PromptSet {
		terminalSpec.Prompt = input.Prompt
	}
	if input.EnvInheritSet {
		terminalSpec.EnvInherit = input.EnvInherit
	}
}

// DefaultShutdownGrace matches Kubernetes terminationGracePeriodSeconds so
// kukeon-injected PID-1 sbsh exits predictably under `kuke stop cell`.
const DefaultShutdownGrace = 30 * time.Second

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
