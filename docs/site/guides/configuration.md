# Configuration

sbsh reads user-level defaults from a single YAML file shaped as a `Configuration` document. The file sets defaults for the run path, profiles directory, and log level; any CLI flag or environment variable still overrides what the document declares.

## File location

By default, both `sbsh` and `sb` look for the config file at:

```
$HOME/.sbsh/config.yaml
```

Override the location with the `--config` flag or the `SBSH_CONFIG_FILE` environment variable.

The file is optional. If it is absent, sbsh falls back to built-in defaults.

## Schema

```yaml
apiVersion: sbsh/v1beta1
kind: Configuration
metadata:
  name: default
spec:
  runPath: /path/to/state-root         # optional, default $HOME/.sbsh/run
  profilesDir: /path/to/profiles.d     # optional, default $HOME/.sbsh/profiles.d
  logLevel: info                       # optional, default info (debug|info|warn|error)
```

All fields under `spec` are optional. An empty or missing field keeps the built-in default.

### Fields

| Field              | Description                                                                                           |
| ------------------ | ----------------------------------------------------------------------------------------------------- |
| `apiVersion`       | Must be `sbsh/v1beta1`.                                                                               |
| `kind`             | Must be `Configuration`. Any other kind is rejected.                                                  |
| `metadata.name`    | Free-form label for the document. Optional.                                                           |
| `spec.runPath`     | Root directory where sbsh writes per-terminal and per-client state.                                   |
| `spec.profilesDir` | Directory scanned recursively for `*.yaml` / `*.yml` files containing `TerminalProfile` documents.    |
| `spec.logLevel`    | Default log level when `--log-level` and `SBSH_LOG_LEVEL` are not provided.                           |

## Precedence

For each setting, the first source that provides a non-empty value wins:

1. CLI flag (e.g. `--run-path`, `--profiles-dir`, `--log-level`)
2. Environment variable (e.g. `SBSH_RUN_PATH`, `SBSH_PROFILES_DIR`, `SBSH_LOG_LEVEL`)
3. `spec` value from `config.yaml`
4. Built-in default

## Example

```yaml
apiVersion: sbsh/v1beta1
kind: Configuration
metadata:
  name: default
spec:
  runPath: /var/lib/sbsh
  profilesDir: /etc/sbsh/profiles.d
  logLevel: debug
```

With this file in place, running `sbsh` without any flags uses `/var/lib/sbsh` as the run path, loads profiles recursively from `/etc/sbsh/profiles.d`, and logs at `debug` level.
