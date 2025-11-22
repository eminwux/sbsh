# sbsh vs screen and tmux

This document compares sbsh with traditional terminal multiplexers (screen and tmux). While all three provide persistent terminal sessions, they differ in architecture, design goals, and use cases.

**screen** and **tmux** are terminal multiplexers: they manage multiple terminal sessions within a single process, providing window management and session persistence.

**sbsh** is a terminal supervisor: it treats terminals as independent, managed services with declarative configuration, built-in discovery, and programmatic control. Unlike screen and tmux, sbsh has no central server process—each terminal runs autonomously. sbsh exists to make terminal environments reproducible, shareable, and automatable across teams and CI/CD pipelines.

## Overview

| Aspect                              | screen / tmux                              | sbsh                                            |
| ----------------------------------- | ------------------------------------------ | ----------------------------------------------- |
| **Server Architecture**             | Single server process manages all sessions | No central server; each terminal is independent |
| **Terminal Process**                | Tied to the terminal multiplexer           | Independent, runs separately                    |
| **Supervisor Model**                | The multiplexer IS the terminal            | Separate supervisor and terminal                |
| **Failure Domain**                  | Shared—server crash affects all sessions   | Isolated—each terminal independent              |
| **Supervisor Crash**                | Terminal typically dies with supervisor    | Terminal continues running                      |
| **Configuration**                   | Dotfiles (`.screenrc`, `.tmux.conf`)       | Declarative YAML profiles                       |
| **Discovery**                       | Manual socket management                   | Built-in discovery by name or ID                |
| **Reattach from Different Machine** | Requires shared socket files / setup       | Built-in discovery, works from anywhere         |
| **Automation**                      | Limited programmatic access                | RPC API, structured metadata                    |
| **Multi-Attach**                    | Requires setup                             | Built-in from the start                         |

## Architecture

**screen / tmux:**

- Single server process manages all terminal sessions
- Sessions are children of the multiplexer process
- Server process crash can affect all sessions
- Sessions share the same failure domain
- Supervisor crash typically terminates the terminal

**sbsh:**

- No central server or daemon process
- Each terminal runs as an independent process with its own lightweight supervisor
- Terminals are self-contained with individual lifecycles, logs, and metadata
- Complete process isolation: one terminal's failure does not affect others
- Supervisor crash does not affect the terminal; it continues running independently

### Benefits of No-Server Architecture

- **Isolated failure domains**: If one supervisor crashes, other terminals continue running
- **True isolation**: Each terminal is a standalone process, not a child of a global daemon
- **Simpler integration**: Terminals can be created, discovered, or attached to individually
- **Scalability**: Run hundreds of terminals without a single point of failure

## Configuration and Profiles

**screen / tmux:**

- Configuration via dotfiles (`.screenrc`, `.tmux.conf`)
- Manual script setup for environments
- Per-terminal setup requires manual commands
- Configuration is procedural, not declarative

**sbsh:**

- **Profiles**: Declarative YAML configuration
- Pre-configured environments (k8s, terraform, docker, etc.)
- Lifecycle hooks (onInit, postAttach)
- Version-controlled, shareable across team members
- Configuration defines desired state, not execution steps

**Example:** With sbsh, a Kubernetes profile automatically sets up context and shows cluster status:

```bash
sbsh -p k8s-default
# Automatically runs: kubectl config use-context, shows pods, etc.
```

With tmux, you'd need custom scripts or manual setup each time.

## Terminal Discovery

**screen / tmux:**

- Manual socket management required
- List sessions with `screen -list` or `tmux ls`
- Reattach requires knowing session name or ID
- Socket files must be accessible for reattachment

```bash
# Manual socket management
screen -S myterminal
screen -list
screen -r myterminal

# Or find socket files manually
ls -la /tmp/screen-*/
```

**sbsh:**

- Built-in discovery by name or ID
- Structured metadata directory with full session information
- Discoverable from any machine with access to the run directory
- No manual socket file management

```bash
# Built-in discovery by name or ID
sb get terminals
sb attach myterminal
sb attach abc123
# Works from any machine, no socket management
```

sbsh maintains a metadata directory (`~/.sbsh/run/terminals/`) with full session information, enabling true session discovery, better observability, and reproducibility across teams.

## Multi-Attach

**screen / tmux:**

- Both support multiple clients attaching to sessions
- Requires explicit multi-user mode setup (screen)
- Or careful socket sharing (tmux)
- Configuration needed for multi-user scenarios

**sbsh:**

- Built-in multi-attach from the start
- Multiple supervisors can connect concurrently to the same terminal
- Designed for collaboration and team workflows
- No special configuration required

## API and Automation

**screen / tmux:**

- Limited programmatic access
- Mostly command-line interface
- Integration requires parsing command output
- No structured API for automation

**sbsh:**

- **RPC API** for programmatic control
- Structured metadata and logs in JSON format
- Designed for automation and CI/CD integration
- Better integration with monitoring and orchestration tools
- Terminal state queryable via API

## Logging and Observability

**screen / tmux:**

- Basic logging (if configured)
- Manual log management
- Limited metadata about sessions
- No structured logging format

**sbsh:**

- **Structured logs** for every terminal
- Complete I/O capture to log files
- Metadata (status, attachers, lifecycle events) in JSON
- Built for auditing and debugging
- Logs stored per-terminal in metadata directory

## When to Use Each

### Use screen / tmux if:

- You need basic terminal persistence
- You want window/panel management (tmux)
- You prefer traditional terminal multiplexing workflows
- Simple use cases are sufficient
- You're comfortable with existing tools and workflows

### Use sbsh if:

- You work with Kubernetes, Terraform, or infrastructure tools
- You need terminal discovery across machines
- You want reproducible environment profiles (Terminal-as-Code)
- You need API access for automation
- You want structured logging and metadata
- Multiple people need to share terminals
- Terminals must survive supervisor crashes
- You need isolated failure domains without process dependencies
- You're running many terminals and need high reliability
- You want autonomous terminals without a central server dependency
- You need declarative configuration that can be version-controlled

## Related Tools

**Ansible** manages infrastructure provisioning and configuration, while **sbsh** focuses on developer session reproducibility. Ansible defines and deploys infrastructure state; sbsh defines and manages terminal environments for consistent developer workflows.

## Philosophy

sbsh treats terminals as first-class, independent resources—each with its own lifecycle, configuration, and failure domain. By eliminating the central server model and embracing declarative configuration, sbsh enables reproducible terminal environments that can be version-controlled, shared across teams, and integrated into automated workflows. This design philosophy prioritizes isolation, observability, and automation over traditional multiplexing features.
