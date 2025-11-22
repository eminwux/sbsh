# Terminals

Terminals are the core abstraction in sbsh. A terminal represents an independent shell environment that runs persistently, separate from any supervisor process.

## Terminal Lifecycle

A terminal goes through several states during its lifecycle:

```
Created → Initializing → Ready → [Attached/Detached] → Exited
```

1. **Created**: Terminal process is spawned
2. **Initializing**: `onInit` hooks are executing
3. **Ready**: Terminal is ready for input (after `onInit` completes)
4. **Attached/Detached**: Terminal can be attached to or detached from supervisors
5. **Exited**: Terminal process has terminated

See [Terminal State](terminal-state.md) for detailed state information.

## Terminal Independence

Terminals run as independent processes with their own process groups. Key characteristics:

- **Process isolation**: Each terminal is a standalone process
- **Survival guarantee**: Terminals continue running even if supervisors exit
- **Metadata persistence**: Terminal state is stored on disk in `~/.sbsh/run/terminals/`
- **Socket-based communication**: Supervisors communicate via Unix domain sockets

## Terminal Discovery

Terminals are discoverable by name or ID:

```bash
# List all terminals
sb get terminals

# Attach by name
sb attach my-terminal

# Attach by ID
sb attach abc123
```

Terminal metadata is stored in `~/.sbsh/run/terminals/<id>/meta.json`, enabling discovery from any machine with access to the run directory.

## Terminal Configuration

Terminals are configured via [Profiles](profiles.md), which specify:

- Command and arguments (`cmd`, `cmdArgs`)
- Working directory (`cwd`)
- Environment variables (`env`)
- Lifecycle hooks (`onInit`, `postAttach`)
- Custom prompts
- Restart policies

## Related Concepts

- [Profiles](profiles.md) - Terminal configuration
- [Supervisor](supervisor.md) - Terminal management
- [Terminal State](terminal-state.md) - State management
- [Multi-Attach](multi-attach.md) - Multiple supervisors
- [Event Log](event-log.md) - I/O capture
