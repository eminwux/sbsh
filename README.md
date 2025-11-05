# 🚂 sbsh - Persistent Terminal Sessions

**Never lose your work when connections drop. Never restart long-running tasks. Never lose your place in a debugging terminal.**

sbsh makes terminals **persistent, discoverable, and resumable**. Your work survives network drops, computer restarts, and accidental disconnects.

**Demo 1 - Launch a terminal with default profile**
[![asciicast](https://asciinema.org/a/chXZV5kG3OYR7gubis6B8nvKE.svg)](https://asciinema.org/a/chXZV5kG3OYR7gubis6B8nvKE)

**Demo 2 - Launch a terminal with a Kubernetes profile**
[![asciicast](https://asciinema.org/a/753754.svg)](https://asciinema.org/a/753754)

**Demo 3 - Launch a terminal with a terraform profile**

[![asciicast](https://asciinema.org/a/vBogwCwOzFm1xChDi1FPDyKpw.svg)](https://asciinema.org/a/vBogwCwOzFm1xChDi1FPDyKpw)

## The Problem You Know

❌ **Lost SSH terminal during debugging?** — All your work is gone

❌ **Terraform plan died on disconnect?** — Start over from scratch

❌ **Can't resume work after laptop sleep?** — Rebuild your entire context

❌ **Need to switch between projects?** — Lose state when you leave

❌ **Collaborate on a terminal?** — Not possible

❌ **Audit what happened in a terminal?** — No structured logs

## The Solution

✅ **Connection drops?** → `sb attach myterminal` → Continue exactly where you left off

✅ **Long-running task?** → Detach, come back hours later, it's still running

✅ **Switch contexts?** → Terminals stay alive, switch anytime

✅ **Share terminals?** → Multiple people can attach to the same terminal

✅ **Full history?** → Structured logs and metadata for every terminal

**sbsh separates the terminal (your work environment) from the supervisor (the controller)**, making terminals as durable and manageable as background services.

## Who Is This For?

**DevOps & Infrastructure Engineers**

- Managing Kubernetes clusters with persistent kubectl terminals
- Running Terraform plans/applies that survive disconnects
- Debugging containers without losing context

**Software Developers**

- Long-running tests, builds, or compilation tasks
- Remote development over unreliable connections
- Multi-project workflows with context switching

**System Administrators & SREs**

- Server management terminals that persist
- Incident response workflows that can't be interrupted
- Auditable, logged terminals for compliance

**Automation Engineers**

- API-driven terminal management for CI/CD
- Scripted operations across persistent terminals
- Tool integration and observability

## Key Benefits

### 🔄 Survive Any Disconnect

Your terminals continue running even when:

- SSH connection drops (remote terminals keep running after local disconnect)
- Network temporarily disconnects (terminal resumes when you reconnect)
- Supervisor process crashes (terminal is independent and keeps running)

**Example:** Start a 2-hour Terraform plan on a remote server via SSH, your local connection drops, come back later: `sb attach terraform-terminal` and it's still running on the remote server.

### 📋 Terminal Discovery & Management

List, find, and reattach to any running terminal:

```bash
$ sb get terminals
ID        NAME           PROFILE        STATUS
bbb4b457  quick_samwise  default        Ready
c2f1a890  k8s-debug      k8s-default    Ready

$ sb attach quick_samwise
# You're back exactly where you left off
```

### 🎯 Profiles for Reproducible Environments

Define once, use everywhere. Profiles configure:

- Commands and arguments
- Environment variables
- Working directories
- Lifecycle hooks (onInit, postAttach)

**Example profiles included:**

- `k8s-default` — Auto-configure kubectl context, show pods on attach
- `terraform-prd` — Select workspace, run init, create plan
- `docker-container` — Persistent container shells
- `ssh-pk` — SSH terminals that survive disconnects

See [examples/profiles/README.md](examples/profiles/README.md) for full documentation.

### 👥 Multi-Attach Support

Multiple supervisors can connect to the same terminal concurrently:

- Pair programming
- Team debugging terminals
- Shared infrastructure terminals

### 📊 Structured Logs & Metadata

Every terminal exposes:

- Complete I/O capture
- Structured metadata
- Event history
- State inspection

Perfect for auditing, debugging, and automation.

### 🔌 Programmable API

Control terminals programmatically for automation:

- Create and manage terminals via API
- Integrate with existing tooling
- Build custom workflows
- Automate operations

## How sbsh Differs from screen and tmux

sbsh, `screen`, and `tmux` all provide persistent terminals, but sbsh is designed for different needs:

### Architecture

| Feature                             | screen / tmux                        | sbsh                                    |
| ----------------------------------- | ------------------------------------ | --------------------------------------- |
| **Terminal Process**                | Tied to the terminal multiplexer     | Independent, runs separately            |
| **Supervisor**                      | The multiplexer IS the terminal      | Separate supervisor and terminal        |
| **After Supervisor Dies**           | Terminal typically dies with it      | Terminal continues running              |
| **Reattach from Different Machine** | Requires shared socket files / setup | Built-in discovery, works from anywhere |

### Terminal Discovery

**screen / tmux:**

```bash
# Manual socket management
screen -S myterminal
screen -list
screen -r myterminal

# Or find socket files manually
ls -la /tmp/screen-*/
```

**sbsh:**

```bash
# Built-in discovery by name or ID
sb get terminals
sb attach myterminal
sb attach abc123
# Works from any machine, no socket management
```

### Configuration and Profiles

**screen / tmux:**

- Configuration via dotfiles (`.screenrc`, `.tmux.conf`)
- Manual script setup for environments
- Per-terminal setup requires manual commands

**sbsh:**

- **Profiles**: Declarative YAML configuration
- Pre-configured environments (k8s, terraform, docker, etc.)
- Lifecycle hooks (onInit, postAttach)
- Reproducible across team members

**Example:** With sbsh, a Kubernetes profile automatically sets up context and shows cluster status:

```bash
sbsh -p k8s-default
# Automatically runs: kubectl config use-context, shows pods, etc.
```

With tmux, you'd need custom scripts or manual setup each time.

### Multi-Attach

**screen / tmux:**

- Both support multiple clients attaching
- Requires explicit multi-user mode setup (screen)
- Or careful socket sharing (tmux)

**sbsh:**

- Built-in multi-attach from the start
- Multiple supervisors can connect concurrently
- Designed for collaboration

### API and Automation

**screen / tmux:**

- Limited programmatic access
- Mostly command-line interface
- Integration requires parsing output or screen/tmux commands

**sbsh:**

- **RPC API** for programmatic control
- Structured metadata and logs
- Designed for automation and CI/CD integration
- Better integration with monitoring tools

### Logging and Observability

**screen / tmux:**

- Basic logging (if configured)
- Manual log management
- Limited metadata

**sbsh:**

- **Structured logs** for every terminal
- Complete I/O capture
- Metadata (status, attachers, lifecycle events)
- Built for auditing and debugging

### Use Cases

**screen / tmux are better for:**

- Simple terminal persistence
- Window/panel management (tmux)
- Quick terminal recovery
- Traditional terminal multiplexing needs

**sbsh is better for:**

- Infrastructure work (k8s, terraform, containers)
- Terminal discovery and management at scale
- Team collaboration and shared terminals
- Automation and CI/CD integration
- Environments that need to survive supervisor restarts
- Auditable, logged terminals

### When to Use Each

**Use screen / tmux if:**

- You just need basic terminal persistence
- You want window/panel management (tmux)
- You're comfortable with existing workflows
- Simple use cases are sufficient

**Use sbsh if:**

- You work with Kubernetes, Terraform, or infrastructure
- You need terminal discovery across machines
- You want reproducible environment profiles
- You need API access for automation
- You want structured logging and metadata
- Multiple people need to share terminals
- Terminals must survive supervisor crashes

### Summary

Think of it this way:

- **screen/tmux**: Terminal multiplexers that happen to persist terminals
- **sbsh**: A terminal supervisor that treats terminals as managed services

sbsh is like having `systemd` for terminals — structured, observable, discoverable, and designed for automation. screen and tmux are like having `nohup` with better UX — simple, effective, but less structured.

Both have their place. Choose sbsh when you need the structure, profiles, discovery, and API access. Choose screen/tmux for traditional multiplexing and simple persistence needs.

## CI/CD Integration

sbsh profiles enable **reproducible environments** that work identically in local development and CI/CD pipelines. Define your environment once, use it everywhere — from pre-commit hooks to GitHub Actions and GitLab CI.

### Local CI/CD Workflows

Use profiles for pre-commit hooks, local testing, and development workflows:

```yaml
# ~/.sbsh/profiles.yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: test-env
spec:
  shell:
    cwd: "~/project"
    env:
      NODE_ENV: "test"
      CI: "true"
  stages:
    onInit:
      - script: docker-compose up -d
      - script: npm install
      - script: npm run test
```

```bash
# .git/hooks/pre-commit
#!/bin/bash
sbsh -p test-env --name "pre-commit-$(date +%s)"
```

### GitHub Actions

Create a profile once, use it in CI. Failed runs can be inspected via persistent terminals:

```yaml
# .github/workflows/test.yml
name: Tests
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Setup sbsh
        run: |
          wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.2.0/sbsh-linux-amd64
          chmod +x sbsh && sudo mv sbsh /usr/local/bin/
      - name: Create test profile
        run: |
          mkdir -p ~/.sbsh
          cat > ~/.sbsh/profiles.yaml <<EOF
          apiVersion: sbsh/v1beta1
          kind: TerminalProfile
          metadata:
            name: ci-test
          spec:
            shell:
              cwd: "${{ github.workspace }}"
              env:
                CI: "true"
            stages:
              onInit:
                - script: npm install
                - script: npm run test
          EOF
      - name: Run tests
        run: sbsh -p ci-test --name "ci-${{ github.run_id }}"
      - name: Upload terminal logs
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: terminal-logs
          path: ~/.sbsh/run/terminals/ci-${{ github.run_id }}/*
```

### GitLab CI

Same profile, different platform. Terminals persist for debugging failed runs:

```yaml
# .gitlab-ci.yml
test:
  script:
    - |
      mkdir -p ~/.sbsh
      cat > ~/.sbsh/profiles.yaml <<EOF
      apiVersion: sbsh/v1beta1
      kind: TerminalProfile
      metadata:
        name: gitlab-test
      spec:
        shell:
          cwd: "$CI_PROJECT_DIR"
          env:
            CI_JOB_ID: "$CI_JOB_ID"
        stages:
          onInit:
            - script: npm install
            - script: npm run test
      EOF
    - sbsh -p gitlab-test --name "gitlab-$CI_JOB_ID"
  artifacts:
    when: always
    paths:
      - ~/.sbsh/run/terminals/gitlab-$CI_JOB_ID/*
    expire_in: 1 week
```

**Key Benefits:**

- **Reproducibility**: Same profile works locally and in CI
- **Debugging**: Failed CI runs create persistent terminals for inspection
- **Version Control**: Profiles are checked into your repo
- **Structured Logs**: Complete I/O capture available as CI artifacts
- **Team Consistency**: Everyone uses the same environment configuration

## Quick Start

### Install

```bash
# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.2.0/sbsh-linux-amd64 && \
chmod +x sbsh && \
sudo mv sbsh /usr/local/bin/ && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

### Autocomplete

```bash
cat >> ~/.bashrc <<EOF
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
EOF
```

## Understanding sbsh Commands

sbsh uses a single binary with hard links (busybox-style) to provide different behaviors:

### `sbsh` - Interactive Supervisor + Terminal

When you run `sbsh` with no arguments, it launches a **supervisor attached to a terminal**. This is designed for interactive use and can be set as your login shell in `/etc/passwd`:

```bash
# Use as login shell
# /etc/passwd
user:x:1000:1000:User:/home/user:/usr/bin/sbsh

# Or run manually
$ sbsh
[sbsh-35af93de] user@host:~/projects$
```

**How it works:**

- Launches a supervisor that stays attached to the terminal
- Internally uses `sbsh run` to create the terminal
- Uses the default profile automatically (unless `-p` is specified)
- You interact directly with the terminal
- Press `Ctrl-]` twice to detach and return to your shell

### `sbsh run` - Terminal Only

The `sbsh run` command launches **just a terminal** (no attached supervisor):

```bash
$ sbsh run --name my-terminal
# Terminal runs in background
# You can attach later with: sb attach my-terminal
```

**How it works:**

- Creates a terminal that runs independently
- Supervisor runs in background (detached mode)
- Terminal continues even after you exit
- Perfect for background tasks, automation, or when you want terminals to persist without an attached supervisor

**Key difference:**

- `sbsh` → Supervisor + Terminal (attached, interactive)
- `sbsh run` → Terminal only (detached, background)

### `sb` - Client Management Tool

The `sb` command is a **pure client tool** that manages existing supervisors and terminals via sockets:

```bash
$ sb get terminals          # List all terminals
$ sb attach myterminal       # Attach to a terminal
$ sb detach                 # Detach from supervisor
$ sb get profiles           # List available profiles
```

**How it works:**

- Only connects to existing supervisors/terminals via Unix sockets
- Never launches new terminals (that's `sbsh`'s job)
- Pure client-side management tool
- Works from any machine that can access the socket files

### Architecture Summary

- **`sbsh`**: Server-side launcher (supervisor + terminal, uses `sbsh run` internally)
- **`sbsh run`**: Terminal-only launcher (can be called directly for detached terminals)
- **`sb`**: Client-side manager (socket-based, never launches terminals)

All three are the same binary accessed via hard links (`ln sbsh sb`), with behavior determined at runtime by the executable name.

## Usage Examples

### Start a Terminal

```bash
$ sbsh
To detach, press ^] twice
[sbsh-35af93de] user@host:~/projects$
```

### Detach (Terminal Keeps Running)

Press `Ctrl-]` twice or run `sb detach`:

```bash
[sbsh-35af93de] user@host:~/projects$ # Press Ctrl-] twice
Detached
$  # Back to your shell, but terminal is still running
```

### List Active Terminals

```bash
$ sb get terminals
ID        NAME           PROFILE  CMD                           STATUS
bbb4b457  quick_samwise  default  /bin/bash --norc --noprofile  Ready
```

### Reattach to a Terminal

```bash
$ sb attach quick_samwise
[sbsh-bbb4b457] user@host:~/projects$ # Back where you left off!
```

### Use a Profile

```bash
# Start with a profile
$ sbsh -p k8s-default
# Automatically configures kubectl context, shows cluster status

# Or use Docker
$ sbsh -p docker-container
sbsh root@container-id:~$
```

### List Available Profiles

```bash
$ sb get profiles
NAME              TARGET  ENVVARS  CMD
k8s-default       local   4 vars   /bin/bash
terraform-prd     local   2 vars   /bin/bash
docker-container  local   0 vars   /usr/bin/docker run ...
ssh-pk            local   0 vars   /usr/bin/ssh -t pk
```

See [examples/profiles/README.md](examples/profiles/README.md) to learn how to create your own profiles.

## Features

- **Persistent terminals** — Survive disconnects and supervisor restarts
- **Terminal discovery** — Find and reattach to any running terminal by ID or name
- **Profiles** — Reproducible environments with commands, env vars, and lifecycle hooks
- **Multi-attach** — Multiple supervisors can connect to the same terminal
- **Structured logs** — Complete I/O capture and metadata for every terminal
- **Programmable API** — Control terminals via API for automation and integration
- **Clean architecture** — Separates terminal (environment) from supervisor (controller)

## How It Works

sbsh creates **independent terminals** that run separately from the supervisor process. When you detach:

1. Your terminal keeps running in the background
2. All I/O is captured and logged
3. Terminal metadata is stored for discovery
4. You can reattach anytime, from anywhere
5. Multiple people can attach to the same terminal

Think of it like `screen` or `tmux`, but with:

- Built-in terminal discovery
- Structured profiles
- API access
- Better separation of concerns

## Why sbsh Exists

Terminals are still treated as ephemeral. Once a shell closes or a connection drops, the environment dies with it.

sbsh changes that by giving terminals persistence and structure. Each sbsh terminal is an independent environment that continues running even if the supervisor exits or restarts. Terminals can be discovered later, reattached, observed, or controlled by new supervisors or API clients.

**sbsh supervises the terminal itself** — the space where people and programs interact — making terminals durable, observable, and programmable.

## Philosophy

sbsh follows the traditional Unix philosophy: build simple tools that do one thing well and compose naturally with others. It treats supervision not as orchestration or complexity, but as clarity — giving interactive work the same discipline that background services have enjoyed for decades.

As Ken Thompson once said, "One of my most productive days was throwing away 1,000 lines of code." sbsh embraces that mindset by keeping its design small, transparent, and essential: a single program that brings persistence and structure to the terminal.

## Status and Roadmap

sbsh is under active development, with a focus on correctness, portability, and clear abstractions before adding integrations.

Work in progress, planned features, and the project roadmap can be found in the [ROADMAP.md](./ROADMAP.md) file.

## Contribute

sbsh is an open project that welcomes thoughtful contributions. The goal is to build a simple, reliable foundation for persistent terminals, not a large framework. Discussions, code reviews, and design proposals are encouraged, especially around clarity, portability, and correctness.

## License

Apache License 2.0

© 2025 Emiliano Spinella (eminwux)
