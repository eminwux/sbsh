# CLI Commands Overview

sbsh provides three main commands through a single binary using hard links (busybox-style):

| Command         | Purpose              | Launches              | Attached      |
| --------------- | -------------------- | --------------------- | ------------- |
| `sbsh`          | Interactive terminal | Client + Terminal     | Yes           |
| `sbsh terminal` | Background terminal  | Terminal only         | No (detached) |
| `sb`            | Management tool      | Nothing (client only) | N/A           |

All three are the same binary; behavior is determined by the executable name at runtime.

## Command Comparison

### `sbsh` - Interactive Client + Terminal

Launches a client attached to a terminal. Designed for interactive use:

```bash
sbsh
sbsh -p profile-name
```

**Features:**

- Client stays attached
- Press `Ctrl-]` twice to detach
- Terminal continues running after detach

### `sbsh terminal` - Terminal Only

Launches a terminal in the background with no attached client:

```bash
sbsh terminal --name my-terminal
sbsh terminal --name my-terminal -p profile-name
```

**Features:**

- Terminal runs independently
- No client attached initially
- Attach later with `sb attach <name>`
- Perfect for automation and background tasks

### `sb` - Management Tool

Pure client tool for managing existing clients and terminals:

```bash
sb get terminals
sb get clients
sb attach <name>
sb detach
sb get profiles
```

**Features:**

- No process spawning
- Works from any machine with socket access
- Query and manage existing terminals

## Common Workflows

### Start and Detach

```bash
# Start interactive terminal
sbsh -p my-profile

# Detach (press Ctrl-] twice)
# Terminal keeps running

# Reattach later
sb attach my-terminal
```

### Background Terminal

```bash
# Start background terminal
sbsh terminal --name background-task -p my-profile

# Attach when needed
sb attach background-task
```

### List and Manage

```bash
# List terminals
sb get terminals

# List clients
sb get clients

# List profiles
sb get profiles

# Get terminal details
sb get terminal my-terminal
```

## See Also

- [sbsh Command](sbsh.md) - Complete `sbsh` reference
- [sb Command](sb.md) - Complete `sb` reference
