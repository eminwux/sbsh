# Process Model

sbsh's process model ensures terminals are independent, isolated, and durable. Unlike traditional terminal multiplexers, sbsh has no central server process.

## No-Server Architecture

### Traditional Multiplexers (screen/tmux)

```
┌─────────────────┐
│  Server Process │  Single process manages all sessions
│  (screen/tmux) │  Sessions are children of server
└────────┬────────┘  Server crash affects all sessions
         │
    ┌────┴────┬─────────┐
    ▼         ▼         ▼
 Session  Session  Session
```

**Problems:**

- Single point of failure
- Shared failure domain
- Server crash affects all sessions

### sbsh Architecture

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│ Supervisor  │  │ Supervisor  │  │ Supervisor  │
│     1       │  │     2       │  │     3       │
└──────┬──────┘  └──────┬──────┘  └──────┬──────┘
       │                │                │
       └────────┬───────┴────────────────┘
                │
         ┌──────┴──────┐
         │   Terminal  │  Independent process
         │  (Process)  │  Not a child of supervisor
         └─────────────┘  Survives supervisor exit
```

**Benefits:**

- No single point of failure
- Isolated failure domains
- Supervisor crashes don't affect terminals
- True process independence

## Process Independence

### Terminal Processes

Terminals run as independent processes:

- Own process group
- Not children of supervisors
- Continue running if supervisor exits
- Can be attached to by new supervisors

### Supervisor Processes

Supervisors are lightweight:

- Manage terminal attachment
- Forward I/O
- Can exit without affecting terminals
- New supervisors can attach to existing terminals

## Process Lifecycle

### Terminal Lifecycle

```
Terminal Process
├── Spawned (independent)
├── Initializing (onInit hooks)
├── Ready (accepting input)
├── Running (executing commands)
└── Exited (or restarted per policy)
```

### Supervisor Lifecycle

```
Supervisor Process
├── Created
├── Attaching (connecting to terminal)
├── Attached (forwarding I/O)
├── Detached (disconnected)
└── Exited (terminal continues)
```

## Isolation Benefits

### Failure Isolation

- Terminal crash doesn't affect other terminals
- Supervisor crash doesn't affect terminals
- No cascading failures
- Each terminal is self-contained

### Resource Isolation

- Each terminal has its own process
- Independent resource limits
- No shared memory or state
- Clean process boundaries

## Multi-Attach Process Model

Multiple supervisors can attach to the same terminal:

```
Terminal Process
├── Supervisor 1 (process A)
├── Supervisor 2 (process B)
└── Supervisor 3 (process C)
```

All supervisors connect via the same Unix domain socket. The terminal broadcasts output to all attached supervisors.

## Process Groups

Terminals run in their own process groups:

- Isolated from parent process
- Can be managed independently
- Proper signal handling
- Clean process tree

## Related Documentation

- [Architecture Overview](overview.md) - System architecture
- [Terminals](../concepts/terminals.md) - Terminal lifecycle
- [Supervisor](../concepts/supervisor.md) - Supervisor architecture
- [Comparison Guide](../guides/comparison.md) - Comparison with screen/tmux
