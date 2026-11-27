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

## Stage Semantics

`onInit` and `postAttach` steps are **typed into the shell's PTY as input**, as if you had typed them at the prompt. sbsh does not exec them as separate subprocesses and does not wait for them to finish.

- **`onInit`** runs once, after the shell has started and the prompt is set up.
- **`postAttach`** runs on every attach, after the terminal is `Ready`.

### How a step is written

For each step, sbsh builds one line, `KEY="value" … script`, and writes it to the PTY followed by a newline. Steps are written in order, about 10ms apart. For example, this step:

```yaml
onInit:
  - script: ./bootstrap.sh
    env:
      FOO: bar
```

is typed into the shell as:

```bash
FOO="bar" ./bootstrap.sh
```

`env` pairs are **inline prefixes, not exports**: they apply only to that command and do not persist in the shell afterwards. To set a variable for the whole session, use `spec.shell.env` or put `export FOO=bar` in a step's `script`.

### Terminal state does not wait for the step

The terminal moves to `OnInit` (or `PostAttach`), the steps are written, and the state moves to `Ready` **right after the last line is written**, not when the commands finish. If an `onInit` step starts a long-running foreground process (a server, `tail -f`), the terminal reports `Ready` while that process is still running. Any later steps are typed into that process's input instead of reaching the shell.

### Caveats

- **Env scoping**: the inline prefix applies only to the first simple command on the line. In `cd app && make`, only `cd` sees the variables. Keys are not written in a fixed order. Values go inside double quotes, so the shell still expands `$`, backticks, and `\` in them.
- **Multi-line scripts**: every newline in `script` works like pressing Enter, so each line runs as a separate command and only the first line gets the `env` prefix. Use one line per step, or call a script file.
- **Here-docs and interactive input**: here-docs work because the shell reads the typed lines like keyboard input, but they show up in the terminal output along with secondary prompts. The same applies to any step that waits for input.
- **Visibility**: steps are echoed in the terminal and in its capture, and an interactive shell may save them to its history.

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
