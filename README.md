# 🚂 sbsh: Terminal-as-Code for reproducible, persistent and shareable shell environments

sbsh brings _Terminal-as-Code_ to your workflow: define terminal environments declaratively with YAML manifests that can be version-controlled, shared, and reused across your team. Each profile specifies environment variables, lifecycle hooks, startup commands, and visual prompts, ensuring consistent setups across local machines, jump hosts, and CI/CD pipelines. Terminals survive network drops, supervisor restarts, and accidental disconnects, remaining discoverable and shareable for collaboration.

**Demo - Launch a Terraform production workspace: profile automatically configures environment, workspace, and shows red warning prompt**

![Demo - Launch a Terraform production workspace: profile automatically configures environment, workspace, and shows red warning prompt](docs/assets/demo-terraform.gif)

## Quick Start

Get sbsh up and running in minutes.

### Install

```bash
# Set your platform (defaults shown)
export OS=linux        # Options: linux, darwin, freebsd
export ARCH=amd64      # Options: amd64, arm64

# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.0/sbsh-${OS}-${ARCH} && \
chmod +x sbsh && \
sudo install -m 0755 sbsh /usr/local/bin/sbsh && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

### Autocomplete

```bash
cat >> ~/.bashrc <<EOF
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
EOF
```

## Why sbsh

_Terminal-as-Code_ brings the discipline of Infrastructure-as-Code to interactive environments:

- Reproducible setups: Profiles ensure identical environments across team members and CI/CD
- Durable terminals: Survive crashes and network interruptions
- Team collaboration: Share terminals with teammates for pair programming and collaborative debugging
- Environment safety: Color-coded prompts and standardized configurations prevent mistakes
- Automation ready: Programmatic API and structured logs enable integration with existing tooling

## Usage Examples

Common workflows for working with terminals and profiles.

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

→ See [docs/profiles/README.md](docs/profiles/README.md) to learn how to create your own profiles.

## Understanding sbsh Commands

Three commands, one binary: sbsh uses hard links (busybox-style) to provide different behaviors.

| Command         | Purpose              | Launches              | Attached      |
| --------------- | -------------------- | --------------------- | ------------- |
| `sbsh`          | Interactive terminal | Supervisor + Terminal | Yes           |
| `sbsh terminal` | Background terminal  | Terminal only         | No (detached) |
| `sb`            | Management client    | Nothing (client only) | N/A           |

All three are the same binary; behavior is determined by the executable name at runtime.

### `sbsh` - Interactive Supervisor + Terminal

Launches a supervisor attached to a terminal. Designed for interactive use and can be set as your login shell:

```bash
$ sbsh
[sbsh-35af93de] user@host:~/projects$
```

Press `Ctrl-]` twice to detach. The terminal keeps running.

### `sbsh terminal` - Terminal Only

Launches a terminal in the background with no attached supervisor:

```bash
$ sbsh terminal --name my-terminal
# Attach later with: sb attach my-terminal
```

Perfect for background tasks and automation.

### `sb` - Client Management Tool

Pure client tool for managing existing supervisors and terminals:

```bash
$ sb get terminals    # List all terminals
$ sb attach <name>    # Attach to a terminal
$ sb detach           # Detach from supervisor
$ sb get profiles     # List available profiles
```

Works from any machine that can access the socket files.

## Profiles: Declarative Environment Manifests

YAML manifests define terminal environments declaratively, the essence of _Terminal-as-Code_. Profiles specify environment variables, lifecycle hooks (`onInit`, `postAttach`), startup commands, and visual prompts. Version-controlled and shared across teams, preventing mistakes like applying Terraform to the wrong workspace or using the wrong Kubernetes cluster.

### Profiles by Example

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

→ See [docs/profiles/README.md](docs/profiles/README.md) for detailed examples and reference.

→ See [docs/features.md](docs/features.md) for a complete overview of features and capabilities.

## How It Works

Terminals run independently from supervisors, defined by profiles and running persistently.

1. Profile defines environment: YAML manifest specifies env vars, commands, hooks, and prompts
2. Terminal runs independently: Shell environment continues even if supervisor exits
3. All I/O captured and logged: Complete audit trail stored for every environment
4. Metadata stored for discovery: Terminal information enables true session discovery
5. Shareable and attachable: Multiple people can attach to the same environment
6. Reattach anytime, from anywhere: Discover and connect to environments by name or ID

## A Tool for Developers & Operations Teams

For teams who need consistent, shareable shell environments. Infrastructure engineers, software developers, DevOps teams, SREs, and automation engineers.

- Infrastructure Engineers: Managing Kubernetes clusters, Terraform workspaces, and cloud resources
- Software Developers: Python, Go, Node.js, Rust development with team-shared configurations
- DevOps Teams: Collaborative debugging, shared infrastructure terminals, standardized CI/CD
- SREs & System Administrators: Persistent server management, incident response workflows
- Automation Engineers: API-driven terminal management, scripted operations, tool integration

## CI/CD Integration

Profiles work identically in local development and CI/CD pipelines. Define your environment once, use it everywhere.

Key benefits: reproducibility (same profile locally and in CI), debugging (persistent terminals for failed runs), version control (profiles in repo), structured logs (complete I/O capture), and team consistency (eliminate setup drift).

→ See [docs/cicd.md](docs/cicd.md) for detailed examples and best practices for GitHub Actions, GitLab CI/CD, and Jenkinsfile.

## Container Usage

Official Docker images for running persistent terminals in containerized environments. Quick deployment in Docker, Kubernetes, or any container orchestration platform.

```bash
docker pull docker.io/eminwux/sbsh:v0.5.0-linux-amd64
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.5.0-linux-amd64 \
  sbsh
```

Images are available for both `linux-amd64` and `linux-arm64` architectures.

→ See [docs/container.md](docs/container.md) for detailed documentation on container usage, volume management, Docker Compose examples, and Kubernetes integration.

## How sbsh Differs from screen and tmux

sbsh is designed for environment management and team collaboration. Key differences: declarative YAML profiles (not dotfiles), built-in discovery and multi-attach, lifecycle hooks, and terminals that survive supervisor crashes.

Unlike tmux or screen, sbsh has no central server or daemon process. Each terminal runs as an independent process with its own lightweight supervisor, so failures are isolated and do not affect other terminals.

→ See [docs/comparison.md](docs/comparison.md) for detailed comparison.

## Why sbsh Exists

Shell environments are still treated as ephemeral and manually configured. Once a shell closes or a connection drops, the environment and all its configuration dies with it. sbsh changes that by making shell environments first-class resources: defined by profiles and durable. Each terminal continues running even if the supervisor exits or restarts.

## Philosophy

sbsh follows the traditional Unix philosophy: build simple tools that do one thing well and compose naturally with others. It treats supervision not as orchestration or complexity, but as clarity, giving interactive work the same discipline that background services have enjoyed for decades. sbsh applies the same principles of Infrastructure-as-Code to interactive environments, turning terminals into reproducible, declarative, and shareable units of work.

As Ken Thompson once said, "One of my most productive days was throwing away 1,000 lines of code." sbsh embraces that mindset by keeping its design small, transparent, and essential: a single program that brings reproducibility, shareability, and persistence to shell environments.

## Status and Roadmap

sbsh is under active development, with a focus on correctness, portability, and clear abstractions before adding integrations.

→ See [ROADMAP.md](./ROADMAP.md) for work in progress, planned features, and the project roadmap.

## Contribute

sbsh is an open project that welcomes thoughtful contributions. The goal is to build a simple, reliable foundation for reproducible, shareable shell environments, not a large framework. Discussions, code reviews, and design proposals are encouraged, especially around clarity, portability, and correctness.

## License

Apache License 2.0

© 2025 Emiliano Spinella (eminwux)
