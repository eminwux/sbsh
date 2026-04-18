# Client

A client is a lightweight process that manages and attaches to terminals. Clients provide the control interface for terminals, handling attachment, detachment, and lifecycle management.

## Client Role

Clients serve several key functions:

- **Terminal management**: Create, attach to, and manage terminals
- **I/O forwarding**: Forward input/output between user and terminal
- **Lifecycle coordination**: Coordinate terminal initialization and hooks
- **Metadata management**: Track terminal state and attachers

## Client Types

### Interactive Client (`sbsh`)

Launches a client attached to a terminal. Designed for interactive use:

```bash
sbsh -p my-profile
```

The client stays attached until you detach (press `Ctrl-]` twice).

### Background Client

Created when you attach to an existing terminal:

```bash
sb attach my-terminal
```

This creates a client that connects to the terminal, then exits when you detach.

## Client Independence

Key architectural principles:

- **No central server**: Each client is independent
- **Terminal survival**: Terminals continue running if the client exits
- **Multi-attach**: Multiple clients can attach to the same terminal
- **Process isolation**: Client crashes don't affect terminals

## Client Lifecycle

```
Created → Attaching → Attached → [Detached] → Exited
```

1. **Created**: Client process spawned
2. **Attaching**: Connecting to terminal socket
3. **Attached**: Connected and forwarding I/O
4. **Detached**: Disconnected but terminal continues
5. **Exited**: Client process terminated

## Client Metadata

Client state is stored in `~/.sbsh/run/clients/<id>/meta.json`, including:

- Terminal ID being managed
- Attachment status
- Creation timestamp
- Process information

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Multi-Attach](multi-attach.md) - Multiple clients
- [Terminal State](terminal-state.md) - State management
