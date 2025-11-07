# Features

sbsh provides a comprehensive set of features for creating reproducible, shareable, and persistent shell environments. Below is an overview of key features, followed by detailed explanations.

## Overview

- **Reproducible shell environments** — Declarative YAML profiles for consistent, shareable environments
- **Team collaboration** — Share environments, multi-attach support, discoverable sessions
- **Persistent terminals** — Survive disconnects and supervisor restarts
- **Terminal discovery** — Find and reattach to any running terminal by ID or name
- **Lifecycle hooks** — Automated setup (`onInit`) and post-attach actions (`postAttach`)
- **Structured logs** — Complete I/O capture and metadata for every terminal
- **Programmable API** — Control terminals via API for automation and integration
- **Clean architecture** — Separates terminal (environment) from supervisor (controller)

## Detailed Features

### Reproducible Environments

Define shell environments once, use everywhere:

- **Declarative profiles**: YAML manifests specify environments, env vars, hooks, and prompts
- **Team consistency**: Share profiles to ensure identical setups across team members
- **CI/CD integration**: Same profiles work locally and in pipelines—test what you ship
- **Environment safety**: Color-coded prompts and standardized configurations prevent mistakes

**Example:** Your team shares a `terraform-prd` profile. Everyone gets the same environment variables, workspace configuration, and red warning prompt—no more applying to the wrong environment.

### Shareable & Discoverable

Share shell environments across your team:

- **Multi-attach**: Multiple people can attach to the same terminal for pair programming and collaborative debugging
- **Discoverable sessions**: Built-in discovery by name or ID—works from any machine without socket management
- **Shared infrastructure terminals**: Team members can access shared debugging and maintenance terminals
- **Metadata directory**: Full session information enables true discovery and observability (not just socket listings)
- **Terminal discovery**: Find and reattach to any running terminal by ID or name from any machine that can access the socket files

**Example:** Start a debugging session, share the terminal name with your team: `sb attach k8s-debug`. Everyone sees the same environment and can collaborate in real-time.

### Persistent & Resumable Terminals

Your shell environments survive disconnects and crashes:

- **SSH connection drops**: Remote terminals keep running after local disconnect
- **Network interruptions**: Terminal resumes when you reconnect
- **Supervisor crashes**: Terminals are independent and keep running
- **Accidental disconnects**: Reattach anytime with `sb attach <name>`

**Example:** Start a 2-hour Terraform plan on a remote server via SSH, your local connection drops, come back later: `sb attach terraform-terminal` and it's still running on the remote server.

### Lifecycle Hooks

Automated setup and post-attach actions for consistent environment initialization:

- **onInit hooks**: Commands that run automatically when a terminal is first created, before the first attach
- **postAttach hooks**: Commands that run automatically every time someone attaches to a terminal
- **Consistent initialization**: Ensure environments are properly configured every time
- **Profile-based configuration**: Define hooks in YAML profiles for reproducibility

**Example:** A Kubernetes profile can use `onInit` to set the kubectl context, and `postAttach` to display current pod status whenever someone attaches.

```yaml
stages:
  onInit:
    - script: kubectl config use-context $KUBE_CONTEXT
  postAttach:
    - script: kubectl get pods
```

### Structured Logs & Metadata

Every shell environment exposes:

- **Complete I/O capture**: Full audit trail of all terminal activity
- **Structured metadata**: Terminal state, environment variables, and configuration
- **Event history**: Lifecycle events and state transitions
- **State inspection**: Query terminal state programmatically

Perfect for auditing, debugging, and automation.

### Programmable API

Control shell environments programmatically:

- **API-driven management**: Create and manage terminals via JSON-RPC API
- **Tool integration**: Integrate with existing tooling and workflows
- **Automation**: Build custom workflows and automated operations
- **CI/CD integration**: Scripted terminal management for pipelines

### Clean Architecture

sbsh separates the terminal (your work environment) from the supervisor (the controller), making terminals as durable and manageable as background services:

- **Process independence**: Terminals run as separate processes with their own process groups
- **Socket-based communication**: Supervisor communicates with terminals via Unix domain sockets
- **No parent dependency**: Terminals don't depend on supervisors being alive
- **Metadata persistence**: Terminal metadata is stored on disk, allowing reconnection
- **Survival guarantee**: Terminals continue running even if supervisors crash or exit
- **Multi-attach support**: Multiple supervisors can attach to the same terminal concurrently

This clean separation means:

- Terminals are independent and survive supervisor crashes
- New supervisors can attach to existing terminals
- Terminals can be discovered and managed without an active supervisor
- Better separation of concerns (environment vs. controller)
