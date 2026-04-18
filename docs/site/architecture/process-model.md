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
│   Client    │  │   Client    │  │   Client    │
│     1       │  │     2       │  │     3       │
└──────┬──────┘  └──────┬──────┘  └──────┬──────┘
       │                │                │
       └────────┬───────┴────────────────┘
                │
         ┌──────┴──────┐
         │   Terminal  │  Independent process
         │  (Process)  │  Not a child of the client
         └─────────────┘  Survives client exit
```

**Benefits:**

- No single point of failure
- Isolated failure domains
- Client crashes don't affect terminals
- True process independence

## Process Independence

### Terminal Processes

Terminals run as independent processes:

- Own process group
- Not children of clients
- Continue running if client exits
- Can be attached to by new clients

### Client Processes

Clients are lightweight:

- Manage terminal attachment
- Forward I/O
- Can exit without affecting terminals
- New clients can attach to existing terminals

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

### Client Lifecycle

```
Client Process
├── Created
├── Attaching (connecting to terminal)
├── Attached (forwarding I/O)
├── Detached (disconnected)
└── Exited (terminal continues)
```

## Isolation Benefits

### Failure Isolation

- Terminal crash doesn't affect other terminals
- Client crash doesn't affect terminals
- No cascading failures
- Each terminal is self-contained

### Resource Isolation

- Each terminal has its own process
- Independent resource limits
- No shared memory or state
- Clean process boundaries

## Multi-Attach Process Model

Multiple clients can attach to the same terminal:

```
Terminal Process
├── Client 1 (process A)
├── Client 2 (process B)
└── Client 3 (process C)
```

All clients connect via the same Unix domain socket. The terminal broadcasts output to all attached clients.

## Process Groups

Terminals run in their own process groups:

- Isolated from parent process
- Can be managed independently
- Proper signal handling
- Clean process tree

## Related Documentation

- [Architecture Overview](overview.md) - System architecture
- [Terminals](../concepts/terminals.md) - Terminal lifecycle
- [Client](../concepts/client.md) - Client architecture
- [Comparison Guide](../guides/comparison.md) - Comparison with screen/tmux
