# Environment Isolation

sbsh provides control over environment variable inheritance and isolation, enabling reproducible environments.

## Environment Inheritance

### `inheritEnv: true` (Default)

When `inheritEnv` is `true`, the terminal inherits all environment variables from the parent process:

```yaml
spec:
  shell:
    inheritEnv: true
    env:
      CUSTOM_VAR: "value"
```

**Use when:**

- You need access to system PATH
- SSH_AUTH_SOCK and other system variables are required
- Full environment access is needed

### `inheritEnv: false`

When `inheritEnv` is `false`, the terminal starts with a minimal environment. Only variables specified in `env` are available:

```yaml
spec:
  shell:
    inheritEnv: false
    env:
      LANG: en_US.UTF-8
      PATH: /usr/bin:/bin
      CUSTOM_VAR: "value"
```

**Use when:**

- You want reproducible, isolated environments
- CI/CD environments that should be clean
- Testing with controlled environments

## Environment Variable Configuration

### Basic Variables

```yaml
spec:
  shell:
    env:
      KEY: value
      NUMBER: "5000" # Numbers must be quoted
      PATH: "$HOME/bin:$PATH" # Can reference other variables
```

### Variable Expansion

Variables can reference other variables:

```yaml
spec:
  shell:
    env:
      HOME: /home/user
      PROJECT_DIR: "$HOME/projects"
      PATH: "$PROJECT_DIR/bin:$PATH"
```

### Special Variables

sbsh provides special variables:

- `$SBSH_TERM_ID`: Current terminal ID
- `$SBSH_TERM_PROFILE`: Current profile name

## Working Directory

Set the working directory with `cwd`:

```yaml
spec:
  shell:
    cwd: "~" # Home directory
    # cwd: "$HOME/projects"  # Expanded path
    # cwd: "/absolute/path"  # Absolute path
```

## Isolation Levels

### Minimal Isolation

```yaml
spec:
  shell:
    inheritEnv: false
    env:
      LANG: en_US.UTF-8
      PATH: /usr/bin:/bin
```

### Full Isolation

```yaml
spec:
  shell:
    inheritEnv: false
    env:
      LANG: en_US.UTF-8
      PATH: /usr/bin:/bin
      HOME: /home/user
      USER: user
      SHELL: /bin/bash
```

### Selective Inheritance

```yaml
spec:
  shell:
    inheritEnv: true
    env:
      NODE_ENV: "production" # Override inherited value
      CUSTOM_VAR: "value" # Add new variable
```

## Best Practices

1. **Use `inheritEnv: false` for CI/CD**: Ensures reproducible environments
2. **Use `inheritEnv: true` for development**: Convenient access to system tools
3. **Quote numbers**: `HISTSIZE: "5000"` not `HISTSIZE: 5000`
4. **Use `$HOME` not `~`**: In env values, use `$HOME` for expansion
5. **Document required variables**: List dependencies in profile comments

## Related Concepts

- [Profiles](profiles.md) - Profile configuration
- [Terminals](terminals.md) - Terminal lifecycle
- [Profiles Guide](../guides/profiles.md) - Complete profile reference
