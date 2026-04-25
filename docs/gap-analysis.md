# sbsh / sb Gap Analysis

Gaps observed while authoring and testing two new profiles (`sbsh-dev`, `kukeon-dev`)
that launch `/bin/bash` and invoke `claude` from `onInit`. Each gap lists the
observed behavior, the impact, a suggested fix, and a rough implementation
complexity. Items are ordered by priority.

## Priority list (highest first)

1. ~~**P0** — `sb get profile <name>` detail view~~ **(DONE in `e09b45e`)**.
2. **P0** — Profile validation / lint command (`sb validate profiles`).
3. ~~**P1** — `sb delete terminal <name>` / `sb stop <name>` for live terminals~~ **(DONE)**.
4. **P1** — Expose terminal PID and cwd in `sb get terminals`.
5. **P1** — `sb logs <name>` with ANSI-stripping option, instead of relying on `--terminal-capture-file`.
6. **P2** — Directory-based profile discovery (`~/.sbsh/profiles.d/*.yaml`).
7. **P2** — Document `onInit` semantics (typed into PTY, not exec'd).
8. **P2** — JSON/YAML output mode for `sb get` (`-o yaml`, `-o json`).
9. **P3** — Published JSON Schema for `TerminalProfile`.
10. **P3** — Name-collision handling on `--terminal-name`.
11. **P3** — Clarify `CREATED` column showing `-` for some terminals.
12. **P3** — Help text tautology: `sbsh --help` says "see sbsh help".

---

## Gaps in detail

### 1. No `sb get profile <name>` detail view  (P0) — **DONE**

Closed by commit `e09b45e` (`feat: wide output for sb get`). At HEAD:
- `sb get profile <name>` works via single-arg dispatch in
  `cmd/sb/get/profiles.go:42-50`; aliases `profile|prof|pro|p`.
- `-o yaml|json` honored at `cmd/sb/get/profiles.go:60` and validated at
  `profiles.go:145-147`; shared marshaler in
  `internal/discovery/terminals.go:399-422`.
- Unit tests in `cmd/sb/get/profiles_test.go`; integration round-trip in
  `cmd/sb/get/get_integration_test.go:431-575`.

Residual polish (tracked informally): `printTerminalMetadata` duplicates
output in the empty-format branch (`%+v` + `PrintHuman`);
`FindProfileByName` returns a plain `fmt.Errorf` rather than a sentinel.

### 2. No profile validation / lint command  (P0)

- **Observed:** An invalid profile (bad YAML, unknown field, invalid
  `runTarget`, malformed `prompt` escape) is only discovered when you try
  to launch it with `sbsh -p <name>`. `sb get profiles` will list it even
  if the spec is nonsense, because it only needs `metadata.name` and `cmd`.
- **Impact:** Errors surface at launch time inside a PTY, making them hard
  to notice (e.g. mis-quoted prompt strings).
- **Fix:** `sb validate profiles [path]` that unmarshals strictly
  (`yaml.UnmarshalStrict` or equivalent), checks enum fields
  (`runTarget`, `restartPolicy`), and verifies `cmd` resolves in PATH when
  `runTarget: local`.
- **Complexity:** Low–medium.

### 3. No way to stop a live terminal  (P1) — **DONE**

Implemented via a new `sb stop terminal <name>` command.

- Command lives in `cmd/sb/stop/` (`stop.go`, `terminals.go`), wired into
  the root at `cmd/sb/sb.go` alongside `prune`.
- Graceful path: new `TerminalController.Stop` RPC (added in
  `pkg/api/terminal.go`, server handler in
  `internal/terminal/terminalrpc/rpc_server.go`, client method in
  `pkg/rpcclient/terminal/rpc_session.go`) triggers
  `controller.Close(…)` asynchronously so the RPC reply can flush
  before teardown.
- Fallback: SIGTERM on `Status.Pid` if the control socket is
  unreachable; `--force` sends SIGKILL immediately;
  `--kill-after` escalates after `--timeout` (default 10s).
- `--all` stops every active terminal; `--ignore-not-found` makes the
  command idempotent. Attached clients are warned then disconnected.
- Metadata is left in place — `sb prune terminals` still owns cleanup.

### 4. `sb get terminals` lacks PID and cwd  (P1)

- **Observed:** Columns are `NAME PROFILE TTY CREATED`. PID only appears in
  the launch output. `cwd` is not shown anywhere post-launch.
- **Impact:** Can't match a terminal row to a running process without extra
  steps. Agents working with many terminals cannot tell them apart if
  profiles/names are similar.
- **Fix:** Add `PID` and `CWD` columns; optionally behind `-o wide` (you
  already do this for `sb get`, per commit `e09b45e`).
- **Complexity:** Low.

### 5. No `sb logs` command; relying on `--terminal-capture-file`  (P1)

- **Observed:** To observe a detached terminal I had to pass
  `--terminal-capture-file /tmp/foo.log` at launch. The file contains raw ANSI
  escapes, control sequences, and bracketed-paste markers, which are hard
  to read.
- **Impact:** Post-hoc debugging of a terminal you didn't start with
  `--terminal-capture-file` is impossible. ANSI noise makes even captured logs
  unreadable without external tooling.
- **Fix:** `sb logs <name> [--follow] [--strip-ansi] [--since <dur>]`.
  Always capture to a per-terminal buffer/file under `run-path`; expose
  via this command.
- **Complexity:** Medium. Requires always-on capture (small buffer or
  on-disk ring).

### 6. Profiles must live in one file  (P2) — RESOLVED

- **Observed (historical):** `~/.sbsh/profiles.yaml` held all profiles separated
  by `---`. Editing one profile touched the shared file; concurrent edits by
  agents/tools risked corruption.
- **Resolution:** Profiles are now loaded from
  `~/.sbsh/profiles.d/` (recursive scan of `*.yaml` / `*.yml`).
  Loader returns warnings instead of aborting on per-file errors, and
  `sb validate profiles` surfaces them for users whose completion looks
  empty. The single-file layout is no longer read — breaking change,
  users migrate by dropping their profiles into `profiles.d/`.

### 7. `onInit` semantics undocumented  (P2)

- **Observed:** `internal/terminal/terminalrunner/lifecycle.go:259-282`
  shows `onInit` steps are typed into the PTY via `sr.Write(...)`. They are
  **not** exec'd as subprocesses. `docs/site/concepts/profiles.md` and the
  example profiles don't make this explicit.
- **Impact:** Users expect shell-level semantics (env from `step.Env`
  exported to the parent shell; script runs before prompt). In reality,
  env key/value pairs are prefixed inline (`FOO="bar" script`), and the
  script inherits whatever the shell's state is at that instant.
  Side-effect: if `onInit` launches a long-lived process like `claude`,
  the terminal state transitions to `Ready` immediately after writing the
  line — it is not "blocked" on the command finishing.
- **Fix:** Add a dedicated section to
  `docs/site/concepts/profiles.md` explaining:
  - `onInit` / `postAttach` are typed into the shell stdin, not exec'd.
  - Foreground vs background behavior.
  - Caveats: multi-line scripts, here-docs, and env scoping.
- **Complexity:** Trivial (docs only).

### 8. No structured output from `sb get`  (P2)

- **Observed:** `sb get profiles` and `sb get terminals` emit a table only.
- **Impact:** Agents cannot reliably parse output; must regex column widths.
- **Fix:** Add `-o json` and `-o yaml` for both commands.
- **Complexity:** Low.

### 9. No published JSON Schema for `TerminalProfile`  (P3)

- **Observed:** I had to cross-reference example profiles
  (`docs/profiles/*.yaml`) and the Go struct to confirm which fields exist
  (e.g., `restartPolicy` enum values: saw `exit`, `restart-on-error`,
  `restart-unlimited` in docs and `on-failure` in user's file — are both
  valid?).
- **Impact:** IDE/editor validation and agent reasoning both suffer.
- **Fix:** Emit JSON Schema from the Go struct (e.g. `invopop/jsonschema`)
  and publish at `docs/schema/terminal-profile.json`.
- **Complexity:** Medium.

### 10. `--terminal-name` collision behavior  (P3)

- **Observed:** Behavior undocumented if a terminal with the same name
  already exists.
- **Impact:** Agents running scripted launches can't predict outcomes.
- **Fix:** Document + either fail fast or append a suffix.
- **Complexity:** Low.

### 11. `CREATED` column sometimes shows `-`  (P3)

- **Observed:** `sb get terminals` showed `CREATED: -` for
  `farseeing_smeagol` while others had `7m`.
- **Impact:** Ambiguous — is it stale? not yet initialized? missing field?
- **Fix:** Either populate reliably or replace `-` with an explicit state
  (`unknown`, `stale`). Consider adding a `STATE` column derived from
  `api.State`.
- **Complexity:** Low.

### 12. Help text tautology  (P3)

- **Observed:** `sbsh --help` prints `You can see available options and
  commands with: sbsh help`.
- **Impact:** Confusing — `sbsh --help` already did that.
- **Fix:** Replace with useful guidance (e.g. link to docs, or list
  example flows).
- **Complexity:** Trivial.

---

## Notes for agents picking this up

- Run `make test` at the repo root before/after any change.
- Profile parsing lives under `pkg/api` and `internal/` (search for
  `TerminalProfile`).
- Table renderers for `sb get` are under `cmd/sb` (commit `e09b45e`
  already introduced wide output — follow the same pattern).
- Lifecycle code (`OnInit`, `PostAttach`, `Close`) is in
  `internal/terminal/terminalrunner/lifecycle.go`.
- `profiles.d/` replaces `~/.sbsh/profiles.yaml` (breaking change).
  The single-file path is no longer read.
