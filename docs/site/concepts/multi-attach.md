# Multi-Attach

sbsh supports multiple supervisors attaching to the same terminal concurrently. This enables collaborative debugging, pair programming, and shared terminal sessions.

## How Multi-Attach Works

Multiple supervisors can connect to the same terminal simultaneously:

```
Terminal (my-terminal)
├── Supervisor 1 (user@host1)
├── Supervisor 2 (user@host2)
└── Supervisor 3 (user@host3)
```

All supervisors see the same terminal output and can send input. Input from any supervisor is forwarded to the terminal.

## Use Cases

### Pair Programming

Two developers can attach to the same terminal:

```bash
# Developer 1
sb attach debug-session

# Developer 2 (from different machine)
sb attach debug-session
```

Both see the same output and can collaborate in real-time.

### Collaborative Debugging

Multiple team members can attach to a shared debugging terminal:

```bash
# Team member 1
sb attach production-debug

# Team member 2
sb attach production-debug

# Team member 3
sb attach production-debug
```

### Shared Infrastructure Terminals

Team members can access shared maintenance terminals:

```bash
# SRE attaches to shared terminal
sb attach k8s-maintenance

# Developer attaches to same terminal
sb attach k8s-maintenance
```

## Multi-Attach Behavior

### Input Handling

Input from any attached supervisor is forwarded to the terminal. If multiple supervisors send input simultaneously, it's interleaved.

### Output Broadcasting

Terminal output is broadcast to all attached supervisors. Each supervisor receives a copy of all output.

### Detachment

When a supervisor detaches, other supervisors remain attached. The terminal continues running independently.

## Discovery Across Machines

Multi-attach works across machines when they share access to the `~/.sbsh` directory:

```bash
# Machine 1
sbsh -p my-profile --name shared-terminal

# Machine 2 (with shared ~/.sbsh)
sb attach shared-terminal
```

## Limitations

- Input from multiple supervisors is interleaved (no coordination)
- All supervisors see the same output (no filtering)
- Requires shared filesystem access for cross-machine multi-attach

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Supervisor](supervisor.md) - Supervisor architecture
- [Terminal State](terminal-state.md) - State management
