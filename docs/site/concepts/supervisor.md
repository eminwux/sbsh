# Supervisor

A supervisor is a lightweight process that manages and attaches to terminals. Supervisors provide the control interface for terminals, handling attachment, detachment, and lifecycle management.

## Supervisor Role

Supervisors serve several key functions:

- **Terminal management**: Create, attach to, and manage terminals
- **I/O forwarding**: Forward input/output between user and terminal
- **Lifecycle coordination**: Coordinate terminal initialization and hooks
- **Metadata management**: Track terminal state and attachers

## Supervisor Types

### Interactive Supervisor (`sbsh`)

Launches a supervisor attached to a terminal. Designed for interactive use:

```bash
sbsh -p my-profile
```

The supervisor stays attached until you detach (press `Ctrl-]` twice).

### Background Supervisor

Created when you attach to an existing terminal:

```bash
sb attach my-terminal
```

This creates a supervisor that connects to the terminal, then exits when you detach.

## Supervisor Independence

Key architectural principles:

- **No central server**: Each supervisor is independent
- **Terminal survival**: Terminals continue running if supervisor exits
- **Multi-attach**: Multiple supervisors can attach to the same terminal
- **Process isolation**: Supervisor crashes don't affect terminals

## Supervisor Lifecycle

```
Created → Attaching → Attached → [Detached] → Exited
```

1. **Created**: Supervisor process spawned
2. **Attaching**: Connecting to terminal socket
3. **Attached**: Connected and forwarding I/O
4. **Detached**: Disconnected but terminal continues
5. **Exited**: Supervisor process terminated

## Supervisor Metadata

Supervisor state is stored in `~/.sbsh/run/supervisors/<id>/meta.json`, including:

- Terminal ID being managed
- Attachment status
- Creation timestamp
- Process information

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Multi-Attach](multi-attach.md) - Multiple supervisors
- [Terminal State](terminal-state.md) - State management
