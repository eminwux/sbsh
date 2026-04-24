# Go library reference

sbsh ships a small public Go library so external programs can spawn,
drive, and tear down `Terminal` and `Client` processes without shelling
out to `sb` / `sbsh` or importing `internal/` packages. This page
documents the supported surface and its stability promise.

A complete, build-tested example lives at
[`docs/examples/library-consumer/`](https://github.com/eminwux/sbsh/tree/main/docs/examples/library-consumer).

## Supported packages

The public library lives under [`github.com/eminwux/sbsh/pkg`](https://pkg.go.dev/github.com/eminwux/sbsh/pkg):

| Package | Purpose |
| --- | --- |
| `pkg/api` | Declarative document types (`TerminalDoc`, `TerminalSpec`, `ClientDoc`, `TerminalProfileDoc`, …) and RPC payload types (`PingMessage`, `WriteRequest`, `SubscribeRequest`, `StopArgs`, …). Counterpart of the YAML/JSON manifests. |
| `pkg/builder` | Construct a `TerminalSpec` or `ClientDoc` from a profile name plus inline overrides (`WithProfile`, `WithProfilesDir`, `WithCommand`, `WithEnv`, `WithClientMode`, …). No YAML round-trip. |
| `pkg/spawn` | Launch detached `Terminal` and `Client` subprocesses (`NewTerminal`, `NewClient`). Returns `TerminalHandle` / `ClientHandle` with `PID`, `SocketPath`, `WaitReady`, `WaitClose`, `Close`. |
| `pkg/discovery` | Enumerate live terminals/clients or look them up by ID/Name under a run path (`ScanTerminals`, `FindTerminalByID`, `FindTerminalByName`, `ScanClients`, `FindClientByName`). Load profiles from disk (`LoadProfilesFromDir`, `FindProfileByNameInDir`). |
| `pkg/rpcclient/terminal` | JSON-RPC client for the `TerminalController` socket (`Ping`, `Resize`, `Attach`, `Detach`, `Metadata`, `State`, `Stop`, `Write`, `Subscribe`). |
| `pkg/rpcclient/client` | JSON-RPC client for the `ClientController` socket (`Ping`, `Metadata`, `State`, `Stop`, `Detach`). |
| `pkg/errors` | Curated error sentinels re-exported from `internal/errdefs`. Use `errors.Is` to branch on well-known failure modes without importing internal packages. |

Nothing under `internal/` is part of the supported surface and
Go's visibility rules prevent external modules from importing it.

## StateRoot contract

Every operation that touches disk derives its paths from a single
**run path** (also called StateRoot in kukeon, `--run-path` on the
CLI). External callers **must** supply this path explicitly — an
empty run path returns `errors.ErrRunPathRequired`. There is no
silent fallback to `$HOME/.sbsh`; the point of the parameter is to
let callers run several isolated sbsh stacks on one host.

Given a run path `R`, the stack writes:

- `R/terminals/<id>/{metadata.json,socket,capture,log}` — one
  directory per spawned terminal.
- `R/clients/<id>/{metadata.json,socket,log}` — one directory
  per spawned client.
- `R/.sbsh/profiles.d/` — the default profile-lookup directory
  when `WithProfilesDir` is not set. Override with
  `WithProfilesDir(myPath)` to point anywhere else.

Library consumers are expected to pass the same run path to every
pkg call in a given operation, and to pass the child processes a
matching `--run-path` flag. `pkg/spawn` does this for you via
`TerminalOptions.ExtraArgs` / `ClientOptions.ExtraArgs`.

## Binary-path contract

`pkg/spawn` deliberately does **not** consult `$PATH`. Callers
must set `TerminalOptions.BinaryPath` / `ClientOptions.BinaryPath`
to an absolute path of an `sbsh` or `sb` binary that supports the
relevant subcommands. This keeps state-root-style isolation from
being silently broken by whatever happens to be first on the
system path.

Both binaries are produced by the canonical `make sbsh-sb` target:
`sbsh` is the real ELF and `sb` is a hard link to it; argv[0]
selects the subtree. Pass the `sbsh` path to `NewTerminal` and the
`sb` path to `NewClient`.

## Pre-v1 stability policy

sbsh is pre-v1. The intent of this policy is to give library
consumers — notably the kukeon integration that drives umbrella
issue [#118](https://github.com/eminwux/sbsh/issues/118) — enough
stability to ship without freezing the surface before v1 has been
validated in anger.

**What will not change between minor releases (0.x → 0.x+1):**

- Wire schema of documents whose `apiVersion` is `sbsh/v1beta1`
  (`TerminalProfileDoc`, `TerminalDoc`, `ClientDoc`, `ConfigurationDoc`).
  Existing fields will not be removed, renamed, or re-typed within
  the v1beta1 apiVersion; new fields may be added.
- The identity of error sentinels re-exported from `pkg/errors`.
  `errors.Is` against those names will continue to match errors
  produced anywhere in sbsh.
- The shape of exported interfaces in `pkg/api`
  (`TerminalController`, `ClientController`): method removals and
  signature-breaking changes will not happen within a minor
  release.

**What may change between minor releases:**

- Option / `With*` function names and defaults in `pkg/builder` and
  `pkg/spawn`. Callers should expect churn as the library ergonomics
  settle.
- Additions to the curated sentinel set in `pkg/errors`.
- Additions to exported interfaces in `pkg/api` (new methods) and
  additions of new request/response payload types.

**What will not change without a deprecation cycle:**

- Removal of any symbol currently documented in `pkg/api`,
  `pkg/errors`, or any subpackage listed above. Removals will be
  staged by marking the symbol deprecated in a minor release and
  removing it no earlier than the next minor release.

v1 will be cut when umbrella issue
[#118](https://github.com/eminwux/sbsh/issues/118) closes and the
library surface has shipped at least one downstream integration.
At v1 the apiVersion advances out of `v1beta1` and the above "may
change" list collapses into "will not change without a deprecation
cycle". See the godoc for
[`pkg/api`](https://pkg.go.dev/github.com/eminwux/sbsh/pkg/api)
for the authoritative copy of this policy.

## Walkthrough

The steps below mirror
[`docs/examples/library-consumer/main.go`](https://github.com/eminwux/sbsh/tree/main/docs/examples/library-consumer).

### 1. Build a `TerminalSpec`

```go
spec, err := builder.BuildTerminalSpec(ctx, logger, stateRoot,
    builder.WithName("library-consumer-example"),
    builder.WithCommand([]string{"/bin/bash", "--norc", "--noprofile"}),
    builder.WithEnv(map[string]string{"PS1": "example> "}),
    builder.WithDisableSetPrompt(true),
)
```

`runPath` is required; everything else has a sane default. Set
`WithProfile(name)` + `WithProfilesDir(dir)` to load from a profiles
directory. Inline `With*` values override profile values.

### 2. Spawn the terminal

```go
term, err := spawn.NewTerminal(ctx, spec, spawn.TerminalOptions{
    BinaryPath:   sbshPath,
    ExtraArgs:    []string{"--run-path", stateRoot},
    Logger:       logger,
    ReadyTimeout: 15 * time.Second,
})
if err := term.WaitReady(ctx); err != nil { /* handle */ }
```

`WaitReady` blocks until the terminal's control socket accepts a
Ping. It returns `ErrProcessExited` if the child dies before
ready, `ErrReadyTimeout` on the internal cap, or `ctx.Err()` on
cancellation.

### 3. Spawn a client that attaches

```go
doc, err := builder.BuildClientDoc(ctx, logger, stateRoot,
    builder.WithClientName("library-consumer-client"),
    builder.WithClientMode(api.AttachToTerminal),
    builder.WithClientTerminalSpec(&api.TerminalSpec{Name: spec.Name}),
    builder.WithClientDetachKeystroke(false),
)

cli, err := spawn.NewClient(ctx, doc, spawn.ClientOptions{
    BinaryPath:   sbPath,
    ExtraArgs:    []string{"--run-path", stateRoot},
    Logger:       logger,
    ReadyTimeout: 15 * time.Second,
})
if err := cli.WaitReady(ctx); err != nil { /* handle */ }
```

### 4. Drive the Terminal RPC surface

```go
trpc := terminalrpc.NewUnix(term.SocketPath(), logger)

// Subscribe BEFORE writing so no output is missed.
stream, err := trpc.Subscribe(ctx,
    &api.SubscribeRequest{ClientID: api.ID("library-consumer")},
    &api.Empty{})
defer stream.Close()

err = trpc.Write(ctx, &api.WriteRequest{Data: []byte("echo hello\n")})
// Read bytes off stream.Read(...) with a deadline.
```

`Subscribe` returns a live `net.Conn` that streams PTY output as raw
bytes — no ANSI stripping, no line framing. Callers typically set
a read deadline and scan for a known marker.

### 5. Drive the Client RPC surface

```go
crpc := clientrpc.NewUnix(cli.SocketPath())
var state api.ClientStatusMode
_ = crpc.State(ctx, &state)            // observe lifecycle
_ = crpc.Stop(ctx, &api.StopArgs{Reason: "done"}) // tear down client
```

### 6. Close the handles

```go
defer term.Close(ctx)
defer cli.Close(ctx)
```

`Close` is idempotent: it sends a Stop RPC, escalates to SIGTERM
and then SIGKILL if needed, and returns the process exit error
(nil on clean exit). When `Stop` has already triggered shutdown
the deferred `Close` becomes a safety net — expect a non-zero
exit status to surface here because the child exited in response
to the Stop signal.

## See also

- [Architecture: process model](../architecture/process-model.md) — how
  Terminal and Client processes relate on the wire.
- [Concepts: client](../concepts/client.md) — what a Client is and
  what `AttachToTerminal` vs `RunNewTerminal` mean.
- [`pkg/api` godoc](https://pkg.go.dev/github.com/eminwux/sbsh/pkg/api) — authoritative type documentation.
