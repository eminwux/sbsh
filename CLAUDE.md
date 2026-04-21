# sbsh

Build and test conventions for anyone — human or agent — working on this repo.

## Build

The canonical build target is `sbsh-sb`. It produces `sbsh` and hardlinks `sb` → `sbsh`:

```makefile
sbsh-sb:
	go build -o sbsh -ldflags="…" ./cmd/
	ln sbsh sb
```

**Do not** build `./cmd/sb` or `./cmd/sbsh` directly — those are library packages, not entry points. `go build -o sb ./cmd/sb` silently produces a `current ar archive` (Go static library), not an ELF binary. `go build` exits 0, so the mistake only surfaces later when exec fails with `exec format error` or `permission denied`.

`./cmd/` is the sole `package main`; subcommand behavior is dispatched via `argv[0]`.

## Verify a build produced an executable

```bash
file ./sbsh  # must be "ELF 64-bit LSB executable", not "current ar archive"
```

## Standard local validation before PR

```bash
make sbsh-sb        # canonical build
go build ./...      # catches non-cmd compile errors
go vet ./...
go test ./<pkg>/... # affected packages
```
