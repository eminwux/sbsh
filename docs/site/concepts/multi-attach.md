# Multi-Attach

sbsh supports multiple clients attaching to the same terminal concurrently. This enables collaborative debugging, pair programming, and shared terminal sessions.

## How Multi-Attach Works

Multiple clients can connect to the same terminal simultaneously:

```
Terminal (my-terminal)
├── Client 1 (user@host1)
├── Client 2 (user@host2)
└── Client 3 (user@host3)
```

All clients see the same terminal output and can send input. Input from any client is forwarded to the terminal.

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

Input from any attached client is forwarded to the terminal. If multiple clients send input simultaneously, it's interleaved.

### Output Broadcasting

Terminal output is broadcast to all attached clients. Each client receives a copy of all output.

### Output Flow Control

sbsh fans a terminal's PTY output to several consumers at once: every
attached interactive client, the on-disk capture file, the live screen
model (used for screenshots and attach repaint), and any passive
`sb read` / `Subscribe` streams. These consumers drain at different
speeds, so the fan-out applies two deliberately different policies when a
consumer cannot keep up:

- **Interactive attachers are flow-controlled (backpressure).** When an
  attached terminal cannot render output as fast as the shell produces
  it, sbsh slows the _producer_ — the shell's `write()` blocks until the
  terminal catches up, exactly as a process is paced by a real PTY. A
  high-throughput command such as `cat /dev/urandom | hexdump` therefore
  scrolls at the terminal's speed and is **never** disconnected for being
  slow.
- **Passive observers are dropped on lag.** A `sb read` / `Subscribe`
  consumer must never be able to stall the live session. If it falls more
  than a bounded amount behind, it is disconnected with a
  `[sbsh: subscriber lagged, disconnecting]` sentinel rather than being
  allowed to apply backpressure.

This split is intentional. The interactive terminal _is_ the session's
output device and should pace the shell; a passive reader is an optional
bystander whose slowness must not freeze the session for everyone else. A
single paused or abandoned attacher likewise must not head-of-line-block
the fan-out and freeze sibling attachers or the capture file. **Do not
collapse interactive attachers onto the drop-on-lag path** — doing so
disconnects a healthy interactive session the moment it runs a firehose.

### Detachment

When a client detaches, other clients remain attached. The terminal continues running independently.

## Discovery Across Machines

Multi-attach works across machines when they share access to the `~/.sbsh` directory:

```bash
# Machine 1
sbsh -p my-profile --name shared-terminal

# Machine 2 (with shared ~/.sbsh)
sb attach shared-terminal
```

## Limitations

- Input from multiple clients is interleaved (no coordination)
- All clients see the same output (no filtering)
- Requires shared filesystem access for cross-machine multi-attach

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Client](client.md) - Client architecture
- [Terminal State](terminal-state.md) - State management
