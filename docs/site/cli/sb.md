# sb Command

The `sb` command is a pure client tool for managing existing clients and terminals. It doesn't spawn any processes itself—it only connects to existing terminals.

## Usage

```bash
sb [command] [flags]
```

## Commands

### `sb attach` (alias: `a`)

Attach to an existing terminal:

```bash
sb attach <name-or-id>
sb attach my-terminal
sb attach abc123
```

**Flags:**

- `--id <id>`: Terminal ID (mutually exclusive with `--name`)
- `--name <name>`: Terminal name (mutually exclusive with `--id`)
- `--socket <file>`: Socket file path

### `sb detach`

Detach from a client:

```bash
sb detach
sb detach <name-or-id>
```

### `sb get`

Query terminals, clients, and profiles:

```bash
# List terminals
sb get terminals

# List clients
sb get clients

# List profiles
sb get profiles

# Get specific terminal
sb get terminal <name-or-id>

# Get specific client
sb get clients <name-or-id>

# Get specific profile
sb get profile <name>
```

**Flags:**

- `-a, --all`: Show all terminals/clients including exited ones
- `-o, --output <format>`: Output format (json, yaml, or human-readable)

### `sb prune`

Prune stale clients and terminals:

```bash
sb prune terminals
sb prune clients
```

A stale client is one that is no longer running. A stale terminal is one whose client is no longer running.

### `sb version`

Show version information:

```bash
sb version
```

### `sb autocomplete`

Generate shell completion:

```bash
sb autocomplete bash
sb autocomplete zsh
```

## Persistent Flags

These flags apply to all `sb` commands:

- `--config <file>`: Config file (default: `$HOME/.sbsh/config.yaml`). See the [Configuration guide](../guides/configuration.md) for the `Configuration` document schema.
- `-v, --verbose`: Enable verbose logging
- `--log-level <level>`: Log level (debug, info, warn, error)
- `--run-path <path>`: Run path directory

## Examples

### List Active Terminals

```bash
sb get terminals
```

### Attach to Terminal by Name

```bash
sb attach my-terminal
```

### Attach to Terminal by ID

```bash
sb attach abc123
```

### List All Terminals (Including Exited)

```bash
sb get terminals -a
```

### Get Terminal Details (JSON)

```bash
sb get terminal my-terminal -o json
```

### Get Profile Details

```bash
sb get profile terraform-prd
```

### Prune Stale Terminals

```bash
sb prune terminals
```

### Prune Stale Clients

```bash
sb prune clients
```

## Output Formats

### Human-Readable (Default)

```bash
sb get terminals
```

### JSON

```bash
sb get terminals -o json
```

### YAML

```bash
sb get terminals -o yaml
```

## See Also

- [sbsh Command](sbsh.md) - Interactive client + terminal
- [Commands Overview](commands.md) - Command comparison
- [Getting Started](../getting-started.md) - Basic usage
