# Terminal State

Terminals transition through several states during their lifecycle. Understanding these states is crucial for managing terminals effectively.

## Terminal States

### Initializing

The terminal is being created and `onInit` hooks are executing. Input is disabled during this state.

**Characteristics:**

- Terminal process is running
- `onInit` hooks are executing
- No input accepted yet
- Client waits for Ready state

### Ready

The terminal has completed initialization and is ready for input. This state is reached after all `onInit` hooks complete successfully.

**Characteristics:**

- Terminal is fully initialized
- Ready to accept input
- Can be attached to
- `postAttach` hooks will run on attach

### Attached

A client is currently connected to the terminal and forwarding I/O.

**Characteristics:**

- Client is connected
- I/O is being forwarded
- User can interact with terminal
- Multiple clients can attach (multi-attach)

### Detached

The terminal is running but no client is currently attached.

**Characteristics:**

- Terminal continues running
- No active client connection
- Can be reattached anytime
- Terminal process is independent

### Exited

The terminal process has terminated.

**Characteristics:**

- Terminal process is dead
- Cannot be attached to
- Metadata preserved for inspection
- May be restarted depending on `restartPolicy`

## State Transitions

```
Created → Initializing → Ready → [Attached/Detached] → Exited
```

### Normal Flow

1. **Created**: Terminal process spawned
2. **Initializing**: `onInit` hooks running
3. **Ready**: Initialization complete
4. **Attached**: Client connects
5. **Detached**: Client disconnects (terminal continues)
6. **Exited**: Terminal process terminates

### With Restart Policy

If `restartPolicy` is set to `restart-on-error` or `restart-unlimited`:

```
Exited → (restart) → Initializing → Ready → ...
```

## Querying Terminal State

```bash
# List terminals with state
sb get terminals

# Get specific terminal info
sb get terminal my-terminal
```

## State Persistence

Terminal state is persisted in `~/.sbsh/run/terminals/<id>/meta.json`, including:

- Current state
- State transitions
- Timestamps
- Attacher information

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Client](client.md) - State management
- [Event Log](event-log.md) - State history
