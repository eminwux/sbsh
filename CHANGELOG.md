# Changelog

All notable user-visible changes to sbsh are recorded here. The format
loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

### Added

- `api.TerminalSpec.MetadataFile` names the metadata document the
  runner writes into `MetadataDir` and reads back for its live-owner
  check (empty keeps `metadata.json`). It must be a plain file name and
  requires `MetadataDir`; a terminal with another name is not listed
  by `sb get` or `pkg/discovery`. Temp files for the atomic write are
  named after it.
- `pkg/terminal/server.WithStatusObserver` delivers a copy of the
  terminal document after every committed status change (state
  transitions, attach, detach, close) on its own goroutine, coalescing
  to the latest document while the observer is busy, so it never
  delays PTY I/O or shutdown. The final `Exited` call may arrive after
  `Serve` returns.
- `api.TerminalStatus` records `exitCode`, `exitSignal` and `exitedAt`
  when the workload exits, in PID-1 init mode and normal mode alike;
  a signaled death sets `exitSignal` (e.g. `SIGKILL`) and leaves
  `exitCode` unset. `Serve` returns a `*server.ExitError` (`Code`,
  `Signal`) for a workload exit, matchable with `errors.As`, with the
  same error text as before.
- Terminals expose read-only I/O activity counters through a new
  `Activity` RPC (`api.TerminalMethodActivity`), reachable via
  `pkg/rpcclient/terminal.Client.Activity` and the
  `pkg/terminal/server` facade. `api.ActivityResult` reports
  `BytesOut` (raw bytes read from the PTY, attached or not),
  `BytesIn` (bytes pushed through `Write`), `LastOutputAt`, a
  terminal-computed `IdleFor`, and `StartedAt`, which marks where the
  in-memory counters were zeroed so consumers can detect a restart.
  Calling it attaches nothing and writes no metadata. A terminal
  started by an older sbsh answers with a method-not-found RPC error.
- `pkg/builder` gained five new `TerminalOption` helpers so SDK
  consumers can populate every shell-shaped field of a `TerminalSpec`
  without writing a YAML profile to disk:
  - `WithStages(api.StagesSpec)` overlays the full lifecycle stages.
  - `WithOnInit([]api.ExecStep)` and `WithPostAttach([]api.ExecStep)`
    overlay only their respective sub-fields.
  - `WithPrompt(string)` stamps `Spec.Prompt`.
  - `WithEnvInherit(bool)` stamps `Spec.EnvInherit` (set-sentinel
    discipline; `false` is a valid distinct value from "unset").
- `pkg/terminal/server` gained client overlays: `ShowToast` /
  `DismissToast` draw a non-modal corner box over live output, and
  `BeginTakeover` returns a `*Takeover` that holds live output for
  chosen clients so the embedder can `Write` full-screen frames, then
  `End` redraws them from the screen model and resumes. `Clients`
  lists attached clients. Overlay bytes reach client connections only,
  never the capture, `Screenshot` or the workload's input; a held
  client's live output is discarded, not buffered, so the workload is
  never slowed. Clients opt out with `api.AttachRequest.NoOverlays` /
  `api.ClientSpec.NoOverlays` / `pkg/attach.Options.DisableOverlays`.
- Attach redraws and `Screenshot.ANSI` now reproduce bold, italic,
  underline, blink and reverse video, not only colors. Colors-only
  output is unchanged.
- `pkg/builder.BuildTerminalSpecFromProfile` preserves the legacy
  profile-driven build path (empty `WithProfile` resolves to
  `"default"`, `pkg/discovery` lookup, hardcoded-default fallback for
  the `"default"` name only).
- Releases now publish `SHA256SUMS` and per-binary `<asset>.sha256`
  checksums (so `install.sh` verifies downloads instead of warning),
  a keyless cosign signature for `SHA256SUMS`, SPDX SBOMs with cosign
  attestations for each binary and container image platform, SLSA
  build provenance for the binaries, and a cosign signature on the
  container image. Verification steps are in
  `docs/site/install/verify.md`. A dedicated `Lint` workflow and README
  badge now report golangci-lint status separately from `Test`.

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
