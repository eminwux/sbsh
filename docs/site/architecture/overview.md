# Architecture Overview

sbsh is built on a clean architecture that separates terminals (work environments) from clients (controllers), making terminals as durable and manageable as background services.

## Core Principles

### Process Independence

Terminals run as independent processes with their own process groups. This means:

- Terminals survive client crashes
- No central server or daemon process
- Each terminal is isolated from others
- Failures are contained to individual terminals

### Separation of Concerns

```
Terminal (Environment)  ←→  Client (Controller)
     ↓                           ↓
  Process                   Management
  I/O Capture              Attachment/Detachment
  State                    Lifecycle Coordination
```

- **Terminal**: The actual shell environment that runs your commands
- **Client**: The lightweight process that manages and attaches to terminals

## Architecture Diagram

```
┌─────────────┐
│   Client    │  Manages and attaches to terminals
│  (sbsh/sb)  │  Handles I/O forwarding
└──────┬──────┘  Lightweight, can exit without affecting terminals
       │
       │ Unix Domain Socket
       │
       ▼
┌─────────────┐
│   Terminal  │  Independent process
│  (Process)  │  Runs shell/command
└─────────────┘  Survives client exit
```

## Key Components

### Terminals

- Independent processes running shell/commands
- Store metadata in `~/.sbsh/run/terminals/<id>/`
- Communicate via Unix domain sockets
- Continue running even if clients exit

### Clients

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
- `~/.sbsh/run/clients/`: Client state
- Enables discovery and reattachment
- Persists across restarts

## Communication Model

### Unix Domain Sockets

Terminals and clients communicate via Unix domain sockets:

- Located in `~/.sbsh/run/terminals/<id>/socket`
- Enables local inter-process communication
- No network overhead
- File-based discovery

### RPC Protocol

Clients use JSON-RPC to communicate with terminals:

- Structured request/response
- State queries
- I/O forwarding
- Lifecycle management

## Failure Model

### Terminal Independence

Terminals are designed to survive:

- Client crashes
- Network disconnections
- Accidental disconnects
- System restarts (with proper volume mounting)

### No Single Point of Failure

- No central server process
- Each terminal is independent
- Client failures don't cascade
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
- [Client](../concepts/client.md) - Client architecture
- [Comparison Guide](../guides/comparison.md) - Comparison with screen/tmux
