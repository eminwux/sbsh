# sbsh vs screen and tmux

sbsh, `screen`, and `tmux` all provide persistent terminals, but sbsh is designed for different needs. This document provides a detailed comparison.

## Architecture

| Feature                             | screen / tmux                        | sbsh                                    |
| ----------------------------------- | ------------------------------------ | --------------------------------------- |
| **Terminal Process**                | Tied to the terminal multiplexer     | Independent, runs separately            |
| **Supervisor**                      | The multiplexer IS the terminal      | Separate supervisor and terminal        |
| **After Supervisor Dies**           | Terminal typically dies with it      | Terminal continues running              |
| **Reattach from Different Machine** | Requires shared socket files / setup | Built-in discovery, works from anywhere |

## Terminal Discovery

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

While screen and tmux only list sockets, sbsh maintains a metadata directory with full session information. This enables true session discovery, better observability, and reproducibility across teams.

## Configuration and Profiles

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

## Multi-Attach

**screen / tmux:**

- Both support multiple clients attaching
- Requires explicit multi-user mode setup (screen)
- Or careful socket sharing (tmux)

**sbsh:**

- Built-in multi-attach from the start
- Multiple supervisors can connect concurrently
- Designed for collaboration

## API and Automation

**screen / tmux:**

- Limited programmatic access
- Mostly command-line interface
- Integration requires parsing output or screen/tmux commands

**sbsh:**

- **RPC API** for programmatic control
- Structured metadata and logs
- Designed for automation and CI/CD integration
- Better integration with monitoring tools

## Logging and Observability

**screen / tmux:**

- Basic logging (if configured)
- Manual log management
- Limited metadata

**sbsh:**

- **Structured logs** for every terminal
- Complete I/O capture
- Metadata (status, attachers, lifecycle events)
- Built for auditing and debugging

## Use Cases

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

## When to Use Each

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

## Summary

Think of it this way:

- **screen/tmux**: Terminal multiplexers that happen to persist terminals
- **sbsh**: A terminal supervisor that treats terminals as managed services

sbsh is like having `systemd` for terminals — structured, observable, discoverable, and designed for automation. screen and tmux are like having `nohup` with better UX — simple, effective, but less structured.

Both have their place. Choose sbsh when you need the structure, profiles, discovery, and API access. Choose screen/tmux for traditional multiplexing and simple persistence needs.

## Related Tools

**Ansible** manages infrastructure provisioning and configuration, while **sbsh** focuses on developer session reproducibility. Ansible defines and deploys infrastructure state; sbsh defines and manages terminal environments for consistent developer workflows.
