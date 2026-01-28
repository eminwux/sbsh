# YAML Schema Reference: TerminalProfile

`sbsh` uses a declarative configuration format. This document defines the schema for a `TerminalProfile`.

## 1. Top-Level Fields
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `apiVersion` | string | Yes | Must be `sbsh/v1beta1`. |
| `kind` | string | Yes | Must be `TerminalProfile`. |
| `metadata` | object | Yes | Contains the `name` of the profile. |
| `spec` | object | Yes | The technical definition of the terminal. |

---

## 2. The `spec` Object
The `spec` defines how the terminal behaves and what it runs.

### `shell` (ShellSpec)
Configures the primary interactive process.
- **`cmd`**: The path to the binary (e.g., `/usr/bin/zsh`).
- **`cmdArgs`**: Arguments passed to the shell (e.g., `["--login"]`).
- **`env`**: Custom environment variables for this session.
- **`cwd`**: The directory the shell starts in.

### `stages` (StagesSpec)
Lifecycle hooks that trigger at specific moments.
- **`onInit`**: A list of `ExecSteps` that run **before** the first user attaches.
- **`postAttach`**: Steps that run every time a user connects.

### `ExecStep`
A single command to run during a lifecycle stage.
- **`script`**: The shell command to execute.
- **`env`**: Environment variables scoped only to this script.