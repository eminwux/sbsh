# sbsh Command

The `sbsh` command launches a supervisor attached to a terminal. It's designed for interactive use and can be set as your login shell.

## Usage

```bash
sbsh [flags]
sbsh terminal [flags]
```

## Commands

### `sbsh` (default)

Launches a supervisor attached to a terminal:

```bash
sbsh
sbsh -p profile-name
```

**Behavior:**

- Supervisor stays attached to terminal
- Press `Ctrl-]` twice to detach
- Terminal continues running after detach

### `sbsh terminal`

Launches a terminal in the background with no attached supervisor:

```bash
sbsh terminal --name my-terminal
sbsh terminal --name my-terminal -p profile-name
```

**Behavior:**

- Terminal runs independently
- No supervisor attached initially
- Attach later with `sb attach <name>`
- **Important**: Always use `--name` to identify the terminal

## Flags

### Persistent Flags

These flags apply to all `sbsh` commands:

- `--run-path <path>`: Optional run path for the supervisor
- `--config <file>`: Config file (default: `$HOME/.sbsh/config.yaml`)
- `--profiles <file>`: Profiles manifests file (default: `$HOME/.sbsh/profiles.yaml`)

### Supervisor Flags

- `--id <id>`: Optional ID for the supervisor
- `--socket <file>`: Optional socket file for the supervisor
- `--log-file <file>`: Optional log file for the supervisor
- `--log-level <level>`: Optional log level (debug, info, warn, error)
- `-d, --detach`: Detach supervisor immediately
- `--disable-detach`: Disable detach keystroke (`Ctrl-]` twice)

### Terminal Flags

- `--terminal-id <id>`: Optional terminal ID (random if omitted)
- `--terminal-name <name>`: Optional name for the terminal
- `-p, --profile <name>`: Profile for the terminal
- `--terminal-command <cmd>`: Command to run (default: `/bin/bash`)
- `--terminal-socket <file>`: Optional socket file for the terminal
- `--capture-file <file>`: Optional filename for the terminal log
- `--terminal-logfile <file>`: Optional filename for the terminal log
- `--terminal-loglevel <level>`: Optional log level for the terminal
- `--terminal-disable-set-prompt`: Disable setting the prompt

## Examples

### Start with Default Profile

```bash
sbsh
```

### Start with Profile

```bash
sbsh -p terraform-prd
```

### Start Background Terminal

```bash
sbsh terminal --name my-terminal -p k8s-default
```

### Start with Custom Command

```bash
sbsh --terminal-command /bin/zsh
```

### Start with Detach Flag

```bash
sbsh -d -p my-profile
```

## Subcommands

### `sbsh terminal`

Launch a terminal without an attached supervisor:

```bash
sbsh terminal --name my-terminal
```

### `sbsh version`

Show version information:

```bash
sbsh version
```

### `sbsh autocomplete`

Generate shell completion:

```bash
sbsh autocomplete bash
sbsh autocomplete zsh
```

## Environment Variables

- `SBSH_PROFILES_FILE`: Path to profiles file (overrides `--profiles`)

## See Also

- [sb Command](sb.md) - Management client
- [Commands Overview](commands.md) - Command comparison
- [Getting Started](../getting-started.md) - Basic usage
