# Configuration

sbsh reads user-level defaults from a single YAML file shaped as a `Configuration` document. The file sets defaults for the run path, profiles file, and log level; any CLI flag or environment variable still overrides what the document declares.

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
  profilesFile: /path/to/profiles.yaml # optional, default $HOME/.sbsh/profiles.yaml
  logLevel: info                       # optional, default info (debug|info|warn|error)
```

All fields under `spec` are optional. An empty or missing field keeps the built-in default.

### Fields

| Field               | Description                                                                 |
| ------------------- | --------------------------------------------------------------------------- |
| `apiVersion`        | Must be `sbsh/v1beta1`.                                                     |
| `kind`              | Must be `Configuration`. Any other kind is rejected.                        |
| `metadata.name`     | Free-form label for the document. Optional.                                 |
| `spec.runPath`      | Root directory where sbsh writes per-terminal and per-client state.         |
| `spec.profilesFile` | Path to the YAML file containing `TerminalProfile` documents.               |
| `spec.logLevel`     | Default log level when `--log-level` and `SBSH_LOG_LEVEL` are not provided. |

## Precedence

For each setting, the first source that provides a non-empty value wins:

1. CLI flag (e.g. `--run-path`, `--profiles`, `--log-level`)
2. Environment variable (e.g. `SBSH_RUN_PATH`, `SBSH_PROFILES_FILE`, `SBSH_LOG_LEVEL`)
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
  profilesFile: /etc/sbsh/profiles.yaml
  logLevel: debug
```

With this file in place, running `sbsh` without any flags uses `/var/lib/sbsh` as the run path, loads profiles from `/etc/sbsh/profiles.yaml`, and logs at `debug` level.
