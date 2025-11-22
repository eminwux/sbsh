# Architecture Overview

sbsh is built on a clean architecture that separates terminals (work environments) from supervisors (controllers), making terminals as durable and manageable as background services.

## Core Principles

### Process Independence

Terminals run as independent processes with their own process groups. This means:

- Terminals survive supervisor crashes
- No central server or daemon process
- Each terminal is isolated from others
- Failures are contained to individual terminals

### Separation of Concerns

```
Terminal (Environment)  ←→  Supervisor (Controller)
     ↓                           ↓
  Process                   Management
  I/O Capture              Attachment/Detachment
  State                    Lifecycle Coordination
```

- **Terminal**: The actual shell environment that runs your commands
- **Supervisor**: The lightweight process that manages and attaches to terminals

## Architecture Diagram

```
┌─────────────┐
│  Supervisor │  Manages and attaches to terminals
│  (sbsh/sb)  │  Handles I/O forwarding
└──────┬──────┘  Lightweight, can exit without affecting terminals
       │
       │ Unix Domain Socket
       │
       ▼
┌─────────────┐
│   Terminal  │  Independent process
│  (Process)  │  Runs shell/command
└─────────────┘  Survives supervisor exit
```

## Key Components

### Terminals

- Independent processes running shell/commands
- Store metadata in `~/.sbsh/run/terminals/<id>/`
- Communicate via Unix domain sockets
- Continue running even if supervisors exit

### Supervisors

- Lightweight management processes
- Handle attachment/detachment
- Forward I/O between user and terminal
- Can attach to existing terminals

### Profiles

- Declarative YAML configuration
- Define terminal environments
- Version-controlled and shareable
- Specify lifecycle hooks and environment

### Metadata Directory

- `~/.sbsh/run/terminals/`: Terminal state and logs
- `~/.sbsh/run/supervisors/`: Supervisor state
- Enables discovery and reattachment
- Persists across restarts

## Communication Model

### Unix Domain Sockets

Terminals and supervisors communicate via Unix domain sockets:

- Located in `~/.sbsh/run/terminals/<id>/socket`
- Enables local inter-process communication
- No network overhead
- File-based discovery

### RPC Protocol

Supervisors use JSON-RPC to communicate with terminals:

- Structured request/response
- State queries
- I/O forwarding
- Lifecycle management

## Failure Model

### Terminal Independence

Terminals are designed to survive:

- Supervisor crashes
- Network disconnections
- Accidental disconnects
- System restarts (with proper volume mounting)

### No Single Point of Failure

- No central server process
- Each terminal is independent
- Supervisor failures don't cascade
- Metadata persists on disk

## Discovery

Terminals are discoverable via metadata directory:

- Structured metadata in JSON
- Query by name or ID
- Works across machines with shared filesystem
- No manual socket management

## Related Documentation

- [Process Model](process-model.md) - Detailed process architecture
- [Terminals](../concepts/terminals.md) - Terminal lifecycle
- [Supervisor](../concepts/supervisor.md) - Supervisor architecture
- [Comparison Guide](../guides/comparison.md) - Comparison with screen/tmux
