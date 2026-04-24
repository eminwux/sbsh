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

Profiles live under `~/.sbsh/profiles.d/` by default. sbsh scans the directory recursively and loads every `*.yaml` / `*.yml` file, so you can keep one profile per file, group related profiles into subdirectories, or combine several into a single multi-document YAML (documents separated by `---`). Point sbsh at a different directory with `--profiles-dir` or `SBSH_PROFILES_DIR`.

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
