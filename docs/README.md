# sbsh Documentation

This directory contains technical documentation for the `sbsh` project, covering architecture, commands, and implementation details.

## Table of Contents

- [Architecture](#architecture)
- [Commands](#commands)
- [Busybox-Style Binary Strategy](#busybox-style-binary-strategy)
- [Profiles](#profiles)
- [Features](./features.md)
- [Container Usage](./container.md)

## Architecture

### Separation of Concerns: Supervisor vs Terminal

sbsh is built on a **clean separation** between two main components:

#### Supervisor

The **supervisor** is the control plane that manages terminals:

- **Lifecycle Management**: Starts, stops, and monitors terminals
- **RPC API Server**: Provides JSON-RPC interface for management operations
- **Connection Management**: Handles multiple clients attaching to terminals
- **Metadata Management**: Tracks supervisor state and terminal relationships
- **Detachment Support**: Allows clients to detach without terminating terminals

Key characteristics:

- Supervisor can exit or crash without affecting running terminals
- Multiple supervisors can attach to the same terminal (multi-attach)
- Supervisor state is stored in `~/.sbsh/run/supervisors/{id}/`

#### Terminal

The **terminal** is the actual running process (shell/command):

- **Independent Process**: Runs separately from the supervisor
- **Persistent Execution**: Continues running even if supervisor exits
- **PTY Management**: Manages pseudo-terminal for the shell/command
- **I/O Capture**: Captures all stdin/stdout for logging and replay
- **RPC Server**: Provides control interface for supervisor communication

Key characteristics:

- Terminal process is independent and survives supervisor crashes
- Terminal state is stored in `~/.sbsh/run/terminals/{id}/`
- Multiple supervisors can connect to the same terminal concurrently
- Terminal continues running until explicitly stopped or the command exits

### Why Terminals Survive Supervisor Crashes

This is the core architectural advantage of sbsh:

1. **Process Independence**: Terminal runs as a separate process with its own process group (`Setsid: true`)
2. **Socket-Based Communication**: Supervisor communicates with terminal via Unix domain sockets
3. **No Parent Dependency**: Terminal doesn't depend on supervisor being alive
4. **Metadata Persistence**: Terminal metadata is stored on disk, allowing reconnection

When a supervisor crashes:

- Terminal process continues running normally
- Terminal's RPC server continues accepting connections
- New supervisor can attach by connecting to the terminal's socket
- All I/O history is preserved in metadata files

### Communication Mechanism

sbsh uses **Unix domain sockets** and **JSON-RPC** for all inter-process communication:

#### Control Socket (Terminal ↔ Supervisor)

**Purpose**: RPC calls for terminal control operations

**Location**: `~/.sbsh/run/terminals/{id}/socket`

**Protocol**: JSON-RPC over Unix domain sockets

**Operations**:

- `Ping`: Health check (supervisor → terminal)
- `Detach`: Detach a supervisor from terminal (supervisor → terminal)
- `Resize`: Terminal resize events (supervisor → terminal)
- `Metadata`: Get terminal metadata (supervisor → terminal)
- `InitTerminal`: Initialize terminal state (supervisor → terminal)

**Client**: Supervisor connects as RPC client
**Server**: Terminal runs RPC server

#### I/O Socket (Terminal ↔ Supervisor)

**Purpose**: Bidirectional stdin/stdout forwarding

**Location**: `~/.sbsh/run/terminals/{id}/io`

**Protocol**: Raw byte stream

**Data Flow**:

- **stdin**: Supervisor → Terminal (user input)
- **stdout/stderr**: Terminal → Supervisor (command output)

**Implementation**: Uses `net.UnixConn` with half-close support (`CloseRead`/`CloseWrite`)

#### Supervisor Control Socket (Supervisor ↔ sb Client)

**Purpose**: RPC calls for supervisor management

**Location**: `~/.sbsh/run/supervisors/{id}/socket`

**Protocol**: JSON-RPC over Unix domain sockets

**Operations**:

- `Detach`: Detach from terminal (sb client → supervisor)
- `Metadata`: Get supervisor metadata (sb client → supervisor)

**Client**: `sb` command connects as RPC client
**Server**: Supervisor runs RPC server

### Process Hierarchy

```
User Terminal
    │
    ├─ sbsh (supervisor process)
    │   ├─ Creates supervisor metadata
    │   ├─ Opens supervisor control socket
    │   ├─ Starts supervisor RPC server
    │   └─ Executes: sbsh terminal --file <terminal-spec.json>
    │       │
    │       └─ Terminal process (sbsh terminal)
    │           ├─ Creates terminal metadata
    │           ├─ Opens terminal control socket
    │           ├─ Opens terminal I/O socket
    │           ├─ Starts terminal RPC server
    │           └─ Executes: <profile.cmd> <profile.cmdArgs>
    │               │
    │               └─ Shell/Command (e.g., /bin/bash)
    │
    └─ sb attach (client process)
        └─ Connects to supervisor control socket
            └─ Supervisor connects to terminal sockets
```

### File Structure

By default, sbsh stores all runtime data in `~/.sbsh/run/`:

```
~/.sbsh/run/
├── supervisors/
│   └── {supervisor-id}/
│       ├── meta.json          # Supervisor metadata (state, PID, etc.)
│       └── socket             # Supervisor control socket (RPC)
│
└── terminals/
    └── {terminal-id}/
        ├── meta.json          # Terminal metadata (state, PID, command, etc.)
        ├── socket             # Terminal control socket (RPC)
        ├── io                 # Terminal I/O socket (stdin/stdout)
        └── capture.log        # I/O capture log (if enabled)
```

**Metadata Files** (`meta.json`):

- Supervisor/terminal state (Ready, Running, Exiting, Exited)
- Process IDs
- Command and arguments
- Environment variables
- Socket file paths
- Creation and update timestamps
- Attached supervisors (for terminals)

**Socket Files**:

- Unix domain sockets for RPC and I/O communication
- Created with appropriate permissions
- Removed when supervisor/terminal exits

## Commands

sbsh provides three main command interfaces through a single binary:

### `sbsh` - Interactive Supervisor + Terminal

**Behavior**: Launches a supervisor attached to a terminal session

**When to use**:

- Interactive terminal sessions
- As a login shell (`/etc/passwd`)
- Daily development work
- When you want immediate access to the terminal

**Usage**:

```bash
# Start with default profile
$ sbsh

# Start with a specific profile
$ sbsh -p k8s-default

# Start with custom name
$ sbsh --name my-terminal
```

**How it works**:

1. Launches a supervisor process
2. Supervisor creates terminal specification from profile
3. Supervisor internally executes `sbsh terminal` with terminal spec
4. Supervisor attaches to the terminal via sockets
5. User interacts directly with the terminal
6. Press `Ctrl-]` twice to detach (terminal keeps running)

**Key characteristics**:

- Supervisor stays attached to terminal
- Uses default profile unless `-p` specified
- Terminal persists after detach
- Can be used as login shell in `/etc/passwd`

### `sbsh terminal` - Terminal Only (Detached)

**Behavior**: Launches a terminal in the background (no attached supervisor)

**When to use**:

- Background tasks
- Automation scripts
- CI/CD pipelines
- Long-running processes
- When you want terminals to run independently

**Usage**:

```bash
# Start detached terminal
$ sbsh terminal --name my-terminal

# Start with profile
$ sbsh terminal -p terraform-prd --name tf-plan

# Start with JSON spec from file
$ sbsh terminal --file terminal-spec.json
```

**How it works**:

1. Creates terminal specification (from profile or flags)
2. Launches terminal process (`sbsh terminal` re-executes itself)
3. Terminal process runs independently
4. Supervisor runs in background (detached mode)
5. Terminal continues even after you exit
6. Attach later with: `sb attach my-terminal`

**Key characteristics**:

- Terminal runs independently
- Supervisor is detached (runs in background)
- Perfect for automation and background tasks
- Terminal persists after process exits

**Difference from `sbsh`**:

- `sbsh` → Supervisor + Terminal (attached, interactive)
- `sbsh terminal` → Terminal only (detached, background)

### `sb` - Client Management Tool

**Behavior**: Pure client tool for managing existing supervisors and terminals

**When to use**:

- Listing terminals/supervisors
- Attaching to existing terminals
- Detaching from terminals
- Managing profiles
- Cleanup operations

**Usage**:

```bash
# List all terminals
$ sb get terminals

# List all supervisors
$ sb get supervisors

# List available profiles
$ sb get profiles

# Attach to a terminal by name
$ sb attach my-terminal

# Attach to a terminal by ID
$ sb attach abc123def

# Detach from current terminal
$ sb detach

# Prune stopped terminals/supervisors
$ sb prune
```

**How it works**:

1. Discovers supervisors/terminals via filesystem scan
2. Connects to supervisor/terminal sockets via RPC
3. Performs operations (attach, detach, get metadata)
4. Never launches new terminals (that's `sbsh`'s job)

**Key characteristics**:

- **Pure client**: Never launches terminals/supervisors
- **Socket-based**: Connects to existing Unix sockets
- **Discovery**: Scans `~/.sbsh/run/` to find terminals/supervisors
- **Multi-machine**: Works from any machine that can access socket files

**Commands**:

- `sb get terminals` - List all running terminals
- `sb get supervisors` - List all running supervisors
- `sb get profiles` - List available profiles
- `sb attach <name|id>` - Attach to a terminal
- `sb detach` - Detach from current terminal
- `sb prune` - Remove stopped terminals/supervisors
- `sb version` - Show version information

### Command Summary

| Command         | Purpose              | Launches              | Attached      |
| --------------- | -------------------- | --------------------- | ------------- |
| `sbsh`          | Interactive terminal | Supervisor + Terminal | Yes           |
| `sbsh terminal` | Background terminal  | Terminal only         | No (detached) |
| `sb`            | Management client    | Nothing (client only) | N/A           |

## Busybox-Style Binary Strategy

sbsh uses a **single binary approach** similar to Busybox, where one compiled binary provides multiple command interfaces through hard links.

### How It Works

#### Single Binary, Multiple Entry Points

Instead of separate binaries, sbsh uses **one compiled binary** with multiple hard links:

```bash
# Installation creates hard link
$ ln sbsh sb

# Both point to same binary
$ ls -li /usr/local/bin/sbsh /usr/local/bin/sb
1234567 -rwxr-xr-x 2 user user 12345678 Jan 1 12:00 /usr/local/bin/sbsh
1234567 -rwxr-xr-x 2 user group 12345678 Jan 1 12:00 /usr/local/bin/sb
```

The `inode` number (1234567) is the same, confirming they're the same file.

#### Runtime Detection

The binary determines its behavior by checking the executable name at runtime:

```go
// cmd/main.go
func main() {
    exe := filepath.Base(os.Args[0])  // Get executable name

    switch exe {
    case "sb":
        // Run sb command tree
        cmd, _ := sb.NewSbRootCmd()
        os.Exit(execRoot(cmd))

    case "sbsh", "-sbsh":
        // Run sbsh command tree
        cmd, _ := sbsh.NewSbshRootCmd()
        os.Exit(execRoot(cmd))

    default:
        // Error or debug mode
        ...
    }
}
```

**Key insight**: `os.Args[0]` contains the executable name, which differs based on how the binary was invoked.

#### Installation

The standard installation process creates the hard link:

```bash
# Set your platform (defaults shown)
export OS=linux        # Options: linux, darwin, freebsd, android(arm64 only)
export ARCH=amd64      # Options: amd64, arm64

# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.1/sbsh-${OS}-${ARCH} && \
chmod +x sbsh && \
sudo mv sbsh /usr/local/bin/ && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

The `ln -f` command creates a hard link, not a symbolic link. Both files point to the same data on disk.

### Benefits

1. **Single Distribution**: One binary to download, update, and maintain
2. **Consistent Versioning**: `sbsh` and `sb` are always the same version
3. **Simpler Installation**: One file, one hard link
4. **Reduced Disk Usage**: Hard links don't duplicate data
5. **Easier Updates**: Update one binary, both commands updated

### Debug Mode

For development and debugging, sbsh supports `SBSH_DEBUG_MODE` environment variable:

```bash
# When running from IDE or with different executable name
$ SBSH_DEBUG_MODE=sb ./my-binary
$ SBSH_DEBUG_MODE=sbsh ./my-binary
```

This allows running sbsh even when the executable isn't named `sbsh` or `sb` (e.g., when running from an IDE or debugger).

### Implementation Details

The main entry point (`cmd/main.go`) handles routing:

1. **Check executable name**: `filepath.Base(os.Args[0])`
2. **Route to appropriate command tree**: `sb.NewSbRootCmd()` or `sbsh.NewSbshRootCmd()`
3. **Execute command**: Use `cobra` to handle command-line parsing and execution
4. **Exit with code**: Return appropriate exit code

Both command trees (`sb` and `sbsh`) are separate `cobra.Command` structures, but they share:

- Common internal packages (`internal/`, `pkg/`)
- Same configuration system
- Same logging infrastructure
- Same error handling

## Profiles

Terminal profiles define how sbsh starts and manages terminals. They configure commands, environment variables, working directories, and lifecycle hooks.

For detailed information about profiles, including:

- Profile structure and fields
- Creating custom profiles
- Lifecycle hooks (`onInit`, `postAttach`)
- Example profiles
- Best practices

See the [Profiles Documentation](./profiles/README.md).

### Quick Profile Overview

Profiles are YAML files that define terminal configurations:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-profile
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cwd: "~/projects"
    cmd: /bin/bash
    env:
      CUSTOM_VAR: "value"
  stages:
    onInit:
      - script: echo "Starting..."
```

Use profiles with the `-p` flag:

```bash
$ sbsh -p my-profile
```

Profiles are loaded from `~/.sbsh/profiles.yaml` by default, or you can specify a custom location with `--profiles-file` or `SBSH_PROFILES_FILE` environment variable.
