# 🚂 sbsh - Persistent Terminal Sessions

**sbsh separates the terminal (your work environment) from the supervisor (the controller)**, making terminals as durable and manageable as background services.

sbsh makes terminals **persistent, discoverable, and resumable**. Your work survives network drops, computer restarts, and accidental disconnects.

sbsh was built to make environment setup **reproducible and shareable**, especially for projects that rely on many environment variables such as Terraform workspaces, Kubernetes clusters, Python virtual environments, and containerized development setups. Using declarative YAML manifests, teams can define terminal environments once and share them to ensure identical setup across local development and CI/CD pipelines. Color-coded prompts for production environments help reduce human error by making it immediately clear which environment you're working in.

**Demo - Launch a terminal with default profile, detach and attach again**
[![asciicast](https://asciinema.org/a/chXZV5kG3OYR7gubis6B8nvKE.svg)](https://asciinema.org/a/chXZV5kG3OYR7gubis6B8nvKE)

## Who Is This For?

**DevOps & Infrastructure Engineers**

- Managing Kubernetes clusters with persistent kubectl terminals
- Running Terraform plans/applies that survive disconnects
- Debugging containers without losing context
- Preventing mistakes through reproducible profiles and visual environment indicators

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

## Profiles: Declarative Environment Manifests

sbsh uses **declarative YAML manifests** to define terminal environments. These manifests specify everything needed for reproducible environments:

- **Environment variables**: Define all required variables (Terraform workspaces, Kubernetes contexts, Python paths, etc.)
- **Commands and startup scripts**: Configure the command to run and any initialization steps
- **Prompts and colors**: Visual indicators that help prevent mistakes (e.g., red prompts for production)
- **Lifecycle hooks**: Automated setup (`onInit`) and post-attach actions (`postAttach`)

These manifests can be shared with teammates to ensure identical environment setup locally and in CI/CD pipelines. Sharing manifests ensures everyone sets up their environment safely and consistently, preventing mistakes like applying Terraform to the wrong workspace or using the wrong Kubernetes cluster.

### Profiles by Example

Profiles define everything needed for reproducible environments. Here are brief examples:

**Terraform production workspace:**

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: terraform-prd
spec:
  shell:
    env:
      TF_VAR_environment: "prd"
    prompt: '"\[\e[1;31m\]sbsh(terraform-prd) \[\e[1;32m\]\u@\h\[\e[0m\]:\w\$ "'
  stages:
    onInit:
      - script: terraform workspace use prd
      - script: terraform init
```

**Kubernetes development terminal:**

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: k8s-default
spec:
  shell:
    env:
      KUBE_CONTEXT: default
      KUBE_NAMESPACE: default
  stages:
    onInit:
      - script: kubectl config use-context $KUBE_CONTEXT
    postAttach:
      - script: kubectl get pods
```

Color-coded prompts and reproducible profiles make environments safer to use by clearly indicating which environment you're working in. See [docs/profiles/README.md](docs/profiles/README.md) for detailed examples and reference.

## Key Benefits

### Survive Any Disconnect

Your terminals continue running even when:

- SSH connection drops (remote terminals keep running after local disconnect)
- Network temporarily disconnects (terminal resumes when you reconnect)
- Supervisor process crashes (terminal is independent and keeps running)

**Example:** Start a 2-hour Terraform plan on a remote server via SSH, your local connection drops, come back later: `sb attach terraform-terminal` and it's still running on the remote server.

### Terminal Discovery & Management

While screen and tmux only list sockets, sbsh maintains a metadata directory with full session information. This enables true session discovery, better observability, and reproducibility across teams.

List, find, and reattach to any running terminal:

```bash
$ sb get terminals
ID        NAME           PROFILE        STATUS
bbb4b457  quick_samwise  default        Ready
c2f1a890  k8s-debug      k8s-default    Ready

$ sb attach quick_samwise
# You're back exactly where you left off
```

### Multi-Attach Support

Multiple supervisors can connect to the same terminal concurrently:

- Pair programming
- Team debugging terminals
- Shared infrastructure terminals

### Structured Logs & Metadata

Every terminal exposes:

- Complete I/O capture
- Structured metadata
- Event history
- State inspection

Perfect for auditing, debugging, and automation.

### Programmable API

Control terminals programmatically for automation:

- Create and manage terminals via API
- Integrate with existing tooling
- Build custom workflows
- Automate operations

## How sbsh Differs from screen and tmux

sbsh, `screen`, and `tmux` all provide persistent terminals, but sbsh is designed for different needs. Key differentiators:

- **Manifests**: sbsh uses declarative YAML manifests for reproducible environments, while screen/tmux rely on dotfiles and manual setup
- **Lifecycle hooks**: Automated setup and post-attach actions with `onInit` and `postAttach`
- **Metadata directory**: Full session information stored in metadata files, enabling true discovery and observability (screen/tmux only list sockets)
- **Structured session discovery**: Built-in discovery by name or ID, works from any machine without socket management
- **Terminal survival**: Terminals continue running even if supervisor crashes (screen/tmux terminals typically die with the multiplexer)

Ansible manages infrastructure provisioning, while sbsh focuses on developer session reproducibility. See [docs/comparison.md](docs/comparison.md) for detailed comparison with screen and tmux.

## CI/CD Integration

sbsh profiles enable **reproducible environments** that work identically in local development and CI/CD pipelines. Define your environment once, use it everywhere — from pre-commit hooks to GitHub Actions, GitLab CI/CD, and Jenkins pipelines.

**Key Benefits:**

- **Reproducibility**: Same profile works locally and in CI — test what you ship
- **Debugging**: Failed CI runs create persistent terminals for inspection — no more "works on my machine"
- **Version Control**: Profiles are checked into your repo — environments as code
- **Structured Logs**: Complete I/O capture available as CI artifacts — full audit trail
- **Team Consistency**: Everyone uses the same environment configuration — eliminate setup drift

See [docs/cicd.md](docs/cicd.md) for detailed examples and best practices for GitHub Actions, GitLab CI/CD, and Jenkinsfile.

## Quick Start

### Install

```bash
# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.4.0/sbsh-linux-amd64 && \
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

## Container Usage

sbsh provides official Docker images for running persistent terminals in containerized environments. Use pre-built images for quick deployment in Docker, Kubernetes, or any container orchestration platform.

**Quick Example:**

```bash
# Pull and run sbsh in a container
docker pull docker.io/eminwux/sbsh:v0.4.0-linux-amd64
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.4.0-linux-amd64 \
  sbsh
```

Images are available for both `linux-amd64` and `linux-arm64` architectures. See [docs/container.md](docs/container.md) for detailed documentation on container usage, volume management, Docker Compose examples, and Kubernetes integration.

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
- Internally uses `sbsh terminal` to create the terminal
- Uses the default profile automatically (unless `-p` is specified)
- You interact directly with the terminal
- Press `Ctrl-]` twice to detach and return to your shell

### `sbsh terminal` - Terminal Only

The `sbsh terminal` command launches **just a terminal** (no attached supervisor):

```bash
$ sbsh terminal --name my-terminal
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
- `sbsh terminal` → Terminal only (detached, background)

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

- **`sbsh`**: Server-side launcher (supervisor + terminal, uses `sbsh terminal` internally)
- **`sbsh terminal`**: Terminal-only launcher (can be called directly for detached terminals)
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

See [docs/profiles/README.md](docs/profiles/README.md) to learn how to create your own profiles.

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
