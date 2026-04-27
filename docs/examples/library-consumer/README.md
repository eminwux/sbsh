# library-consumer

Minimal end-to-end demo of driving `sbsh` as a Go library from an external
module.

## What it does

The example always spawns a detached `Terminal` subprocess via
`pkg/spawn.NewTerminal`, then exercises one of two consumer-facing
façades against it (selected with `-mode`):

### `-mode=rpc` (default)

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

### `-mode=attach`

1. Same `TerminalSpec` build + `pkg/spawn.NewTerminal` spawn as above.
2. Resolves the terminal's control socket via the spawn handle
   (`spawn.TerminalHandle.SocketPath`).
3. Calls `pkg/attach.Run` with that socket and the process's
   TTY-backed stdio (`os.Stdin` / `os.Stdout` / `os.Stderr`). The
   call blocks until the user detaches (`^]^]`), the remote terminal
   closes, or `SIGINT` / `SIGTERM` cancels the context.
4. Closes the terminal handle as a deferred safety net.

This mode is the recommended entry point for embedders that want an
in-process attach loop (e.g. external CLIs like `kuke attach`) — no
`sb` subprocess is involved.

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
# RPC mode (default): scripted Write/Read + State/Stop round-trip
go run . -sbsh "$(pwd)/../../../sbsh" -v

# Attach mode: hands you an interactive shell over pkg/attach.Run.
# Detach with ^] twice, or send SIGINT/SIGTERM to exit.
go run . -mode attach -sbsh "$(pwd)/../../../sbsh" -v
```

Flags:

| Flag | Description |
| --- | --- |
| `-sbsh` | **Required.** Absolute path to the `sbsh` binary. |
| `-sb` | Absolute path to the `sb` binary. Defaults to `$(dirname <sbsh>)/sb`. Only consulted in `-mode=rpc`. |
| `-mode` | `rpc` (default) or `attach`. See "What it does" above. |
| `-state-root` | Run path for sbsh state. Defaults to a fresh `$TMPDIR/sbsh-lib-example-*` directory (cleaned up on exit). |
| `-v` | Log spawn/RPC internals at debug level. |

### Headless contexts (`-mode=attach`)

`pkg/attach.Run` requires `Options.Stdin` to be a TTY-backed `*os.File` —
it puts the fd in raw mode and reads window size via `TIOCGWINSZ`. When
running interactively (the `go run .` invocation above), the process
inherits the user's terminal and the requirement is satisfied.

For non-interactive contexts (CI runners, integration tests, daemons),
allocate a PTY pair and pass the slave end as `Stdin`/`Stdout` —
[`github.com/creack/pty`](https://pkg.go.dev/github.com/creack/pty) is
the idiomatic choice. This example deliberately does not pull that
dependency in, so the headless path lives in the embedder's harness.

## Why a separate module?

A separate `go.mod` with `replace github.com/eminwux/sbsh => ../../../`
guarantees the example only compiles against the library's public
packages (`pkg/...`) — internal packages cannot leak in, because
Go's visibility rules prevent external modules from importing
`internal/`. The CI job build-tests this separate module so the
public surface cannot silently rot.
