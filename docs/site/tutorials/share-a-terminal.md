# Share a Terminal

This tutorial shows you how to share terminals with your team for collaborative debugging and pair programming.

## Prerequisites

- sbsh installed
- Access to shared filesystem (for cross-machine sharing)

## Step 1: Start a Named Terminal

Start a terminal with a descriptive name:

```bash
sbsh -p my-profile --terminal-name shared-debug
```

Or use the short form:

```bash
sbsh -p my-profile --name shared-debug
```

The terminal is now running and can be discovered by name.

## Step 2: List Terminals

From another session (or machine), list available terminals:

```bash
sb get terminals
```

You should see your `shared-debug` terminal in the list.

## Step 3: Attach from Another Session

Another team member can attach to the same terminal:

```bash
sb attach shared-debug
```

Both users now see the same terminal output and can send input.

## Step 4: Multi-Attach Demonstration

Have multiple team members attach:

```bash
# Team member 1
sb attach shared-debug

# Team member 2 (from different machine)
sb attach shared-debug

# Team member 3
sb attach shared-debug
```

All three see the same output in real-time.

## Step 5: Detach and Reattach

Detach from the terminal (press `Ctrl-]` twice or run `sb detach`):

```bash
sb detach
```

The terminal continues running. Reattach anytime:

```bash
sb attach shared-debug
```

## Cross-Machine Sharing

For sharing across machines, ensure all machines have access to the same `~/.sbsh` directory:

### Option 1: Shared Network Filesystem

Mount a shared directory:

```bash
# Machine 1
export SBSH_RUN_PATH=/shared/sbsh
sbsh -p my-profile --name shared-terminal

# Machine 2 (with same mount)
export SBSH_RUN_PATH=/shared/sbsh
sb attach shared-terminal
```

### Option 2: SSH with Shared Home

If machines share the same home directory (e.g., via NFS):

```bash
# Machine 1
sbsh -p my-profile --name shared-terminal

# Machine 2 (same user, shared home)
sb attach shared-terminal
```

## Use Cases

### Pair Programming

Two developers can work together:

```bash
# Developer 1 starts terminal
sbsh -p dev-env --name pair-programming

# Developer 2 attaches
sb attach pair-programming
```

### Collaborative Debugging

Multiple team members debug together:

```bash
# SRE starts debugging terminal
sbsh -p production --name prod-debug

# Developers attach
sb attach prod-debug
```

### Shared Infrastructure Terminals

Team members access shared maintenance terminals:

```bash
# Start shared terminal
sbsh -p k8s-maintenance --name k8s-shared

# Team members attach as needed
sb attach k8s-shared
```

## Best Practices

1. **Use descriptive names**: `shared-debug` is better than `term1`
2. **Document shared terminals**: Let your team know which terminals are shared
3. **Coordinate input**: When multiple people are attached, coordinate who sends input
4. **Clean up**: Prune old shared terminals regularly

## Related Concepts

- [Multi-Attach](../concepts/multi-attach.md) - How multi-attach works
- [Terminals](../concepts/terminals.md) - Terminal lifecycle
- [Supervisor](../concepts/supervisor.md) - Supervisor architecture
