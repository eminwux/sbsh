# Profiles

Profiles are declarative YAML manifests that define terminal environments. They specify environment variables, lifecycle hooks, startup commands, and visual prompts.

## Profile Structure

Each profile is a YAML document with the following structure:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: profile-name
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cwd: "~"
    cmd: /bin/bash
    cmdArgs: []
    inheritEnv: true
    env:
      KEY: value
    prompt: "custom prompt"
  stages:
    onInit:
      - script: echo "Starting..."
    postAttach:
      - script: echo "Attached!"
```

## Profile Components

### Metadata

- **`name`**: Unique identifier for the profile (required)

### Spec

- **`runTarget`**: Where the terminal runs (currently only `local`)
- **`restartPolicy`**: Behavior on exit (`exit`, `restart-on-error`, `restart-unlimited`)
- **`shell`**: Command and environment configuration
- **`stages`**: Lifecycle hooks (`onInit`, `postAttach`)

## Profile Storage

Profiles are stored in `~/.sbsh/profiles.yaml` by default. Multiple profiles are defined in the same file, separated by `---` (YAML document separator).

## Profile Discovery

List available profiles:

```bash
sb get profiles
```

## Using Profiles

Start a terminal with a profile:

```bash
sbsh -p profile-name
```

## Profile Examples

See the [Profiles Guide](../guides/profiles.md) for comprehensive examples and reference documentation.

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Environment Isolation](environment-isolation.md) - Environment configuration
- [Profiles Guide](../guides/profiles.md) - Complete profile reference
