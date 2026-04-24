# library-consumer

Minimal end-to-end demo of driving `sbsh` as a Go library from an external
module.

## What it does

1. Builds a `TerminalSpec` inline via `pkg/builder` with a caller-supplied
   `StateRoot` (no writes under `$HOME/.sbsh`).
2. Spawns a detached `Terminal` subprocess via `pkg/spawn.NewTerminal`.
3. Builds a `ClientDoc` in `AttachToTerminal` mode and spawns a `Client`
   via `pkg/spawn.NewClient`.
4. Drives the Terminal RPC surface via `pkg/rpcclient/terminal`:
   `Subscribe` for live output, `Write` to inject a shell command,
   then reads the marker back off the subscribe stream.
5. Drives the Client RPC surface via `pkg/rpcclient/client`:
   `State` to observe the client's lifecycle, then `Stop` to tear it
   down cleanly.
6. Closes both handles (`spawn.TerminalHandle.Close` /
   `spawn.ClientHandle.Close`) as a deferred safety net.

## Building

The example is a **separate Go module** with a `replace` directive that
points back at the repo root. Build it standalone:

```bash
cd docs/examples/library-consumer
go build ./...
```

## Running

You need the canonical `sbsh` + `sb` hardlink pair built first. From the
repo root:

```bash
make sbsh-sb
```

Then run the example, pointing it at those binaries:

```bash
cd docs/examples/library-consumer
go run . -sbsh "$(pwd)/../../../sbsh" -v
```

Flags:

| Flag | Description |
| --- | --- |
| `-sbsh` | **Required.** Absolute path to the `sbsh` binary. |
| `-sb` | Absolute path to the `sb` binary. Defaults to `$(dirname <sbsh>)/sb`. |
| `-state-root` | Run path for sbsh state. Defaults to a fresh `$TMPDIR/sbsh-lib-example-*` directory (cleaned up on exit). |
| `-v` | Log spawn/RPC internals at debug level. |

## Why a separate module?

A separate `go.mod` with `replace github.com/eminwux/sbsh => ../../../`
guarantees the example only compiles against the library's public
packages (`pkg/...`) — internal packages cannot leak in, because
Go's visibility rules prevent external modules from importing
`internal/`. The CI job build-tests this separate module so the
public surface cannot silently rot.
