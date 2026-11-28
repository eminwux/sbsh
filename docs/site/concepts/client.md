# Client

A client is a lightweight process that manages and attaches to terminals. Clients provide the control interface for terminals, handling attachment, detachment, and lifecycle management.

## Client Role

Clients serve several key functions:

- **Terminal management**: Create, attach to, and manage terminals
- **I/O forwarding**: Forward input/output between user and terminal
- **Lifecycle coordination**: Coordinate terminal initialization and hooks
- **Metadata management**: Track terminal state and attachers

## Client Types

### Interactive Client (`sbsh`)

Launches a client attached to a terminal. Designed for interactive use:

```bash
sbsh -p my-profile
```

The client stays attached until you detach (press `Ctrl-]` twice).

### Background Client

Created when you attach to an existing terminal:

```bash
sb attach my-terminal
```

This creates a client that connects to the terminal, then exits when you detach.

## Client Independence

Key architectural principles:

- **No central server**: Each client is independent
- **Terminal survival**: Terminals continue running if the client exits
- **Multi-attach**: Multiple clients can attach to the same terminal
- **Process isolation**: Client crashes don't affect terminals

## Client Lifecycle

```
Created → Attaching → Attached → [Detached] → Exited
```

1. **Created**: Client process spawned
2. **Attaching**: Connecting to terminal socket
3. **Attached**: Connected and forwarding I/O
4. **Detached**: Disconnected but terminal continues
5. **Exited**: Client process terminated

## Client Metadata

Client state is stored in `~/.sbsh/run/clients/<id>/meta.json`, including:

- Terminal ID being managed
- Attachment status
- Creation timestamp
- Process information

## Client Overlays

A program that embeds sbsh through `pkg/terminal/server` can show
notices to attached interactive clients without writing into the
workload's terminal. sbsh adds no RPC or keystroke for this: only the
embedder starts an overlay, in response to its own events.

- **Toast**: a small box in a screen corner. Output keeps flowing and
  keystrokes reach the workload. sbsh redraws the box after every chunk
  of output, puts the cursor and text style back, re-places it on
  resize, and repaints the area underneath when the toast expires or is
  dismissed.
- **Takeover**: a full-screen notice or animation. Live output is held
  for the chosen clients while the embedder writes frames, then each
  client is redrawn from the current screen and live output resumes.
  Keystrokes are dropped during a takeover by default, or passed to the
  workload. The detach keystroke (`Ctrl-]` twice) is detected by the
  client, so it keeps working.

What overlays guarantee:

- Overlay bytes go to client connections only. The capture file,
  `sb screenshot` and the workload's input never see them.
- A held client's live output is discarded, not buffered, so a takeover
  never slows the workload, the capture or other attached clients.
- Overlay bytes are never inserted inside an escape sequence. A switch
  waits for the output to reach the end of the current sequence, for at
  most 100 ms. After that, sbsh aborts the client's partial sequence
  with CAN (0x18) and repaints the client from the screen model.
- Redraws use absolute cursor moves inside synchronized output
  (`CSI ?2026h` … `CSI ?2026l`) and never touch the workload's saved
  cursor (no DECSC/DECRC).

A client that draws its own notices, for example a web UI showing HTML
toasts, opts out with `api.AttachRequest.NoOverlays` (or
`pkg/attach.Options.DisableOverlays`) and then receives no overlay
bytes. See [the Go library reference](../reference/library.md#client-overlays)
for the API.

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Multi-Attach](multi-attach.md) - Multiple clients
- [Terminal State](terminal-state.md) - State management
