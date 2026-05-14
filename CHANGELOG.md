# Changelog

All notable user-visible changes to sbsh are recorded here. The format
loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

### Added

- `pkg/builder` gained five new `TerminalOption` helpers so SDK
  consumers can populate every shell-shaped field of a `TerminalSpec`
  without writing a YAML profile to disk:
  - `WithStages(api.StagesSpec)` overlays the full lifecycle stages.
  - `WithOnInit([]api.ExecStep)` and `WithPostAttach([]api.ExecStep)`
    overlay only their respective sub-fields.
  - `WithPrompt(string)` stamps `Spec.Prompt`.
  - `WithEnvInherit(bool)` stamps `Spec.EnvInherit` (set-sentinel
    discipline; `false` is a valid distinct value from "unset").
- `pkg/builder.BuildTerminalSpecFromProfile` preserves the legacy
  profile-driven build path (empty `WithProfile` resolves to
  `"default"`, `pkg/discovery` lookup, hardcoded-default fallback for
  the `"default"` name only).

### Changed

- **Breaking:** `pkg/builder.BuildTerminalSpec` now builds a
  `TerminalSpec` from inline options only. It no longer calls
  `pkg/discovery`, no longer resolves an implicit `"default"`
  profile, and no longer falls back to the hardcoded default profile.
  Stages / Prompt / EnvInherit stay zero unless the matching `With*`
  option is passed; Cmd / CmdArgs default to `/bin/bash -i` so the
  runner has a sensible shell.
- **Breaking:** Passing `WithProfile` or `WithProfilesDir` to the
  inline `BuildTerminalSpec` returns `errdefs.ErrInvalidOption`. Use
  `BuildTerminalSpecFromProfile` instead.

### Migration

- In-tree CLI / test callers that previously relied on the implicit
  profile resolution should switch to `BuildTerminalSpecFromProfile`.
  The `cmd/sbsh` entry points have already been migrated.
- Out-of-tree SDK callers (e.g., kukeon's `kuketty`) should keep using
  `BuildTerminalSpec` for the inline lane and adopt `WithPrompt` /
  `WithStages` / `WithOnInit` / `WithPostAttach` / `WithEnvInherit`
  instead of relying on `WithDisableSetPrompt(true)` as a safety belt
  against the old hardcoded prompt.
