# Terminal Profiles for sbsh

Profiles define how sbsh starts and manages terminals. They let you customize the command, environment, working directory, and lifecycle hooks for your terminals.

## Quick Start

### Where to put your profiles

sbsh looks for profiles in `$HOME/.sbsh/profiles.yaml` by default. You can also specify a different file using:

- Environment variable: `SBSH_PROFILES_FILE=/path/to/profiles.yaml`
- Command flag: `sbsh --profiles-file /path/to/profiles.yaml`

### Create your first profile

1. Create the profiles directory (if it doesn't exist):

   ```bash
   mkdir -p ~/.sbsh
   ```

2. Copy example profiles to get started:

   ```bash
   # Copy the combined profiles file
   cp docs/profiles/profiles.yaml ~/.sbsh/profiles.yaml

   # Or combine individual example files (provided for reference)
   cat docs/profiles/*.yaml > ~/.sbsh/profiles.yaml
   ```

   **Note**: sbsh only supports a single `profiles.yaml` file. The individual `.yaml` files in `docs/profiles/` are provided as reference examples, but you must combine them into one file for use.

3. Edit `~/.sbsh/profiles.yaml` and add your own profiles.

### Minimal example

Here's the simplest profile you can create:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-terminal
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cmd: /bin/bash
```

To use it, run:

```bash
sbsh -p my-terminal
```

That's it! This will start a bash terminal with default settings.

## Profile Structure

Each profile is a YAML document (multiple profiles are separated by `---`). Here's the complete structure:

```yaml
apiVersion: sbsh/v1beta1 # Always use this version
kind: TerminalProfile # Always use this kind
metadata:
  name: profile-name # Your profile identifier
spec:
  runTarget: local # Where to run (currently only "local")
  restartPolicy: exit # What happens on exit
  shell: # The command and environment
    cwd: "~" # Working directory
    cmd: /bin/bash # Command to run
    cmdArgs: [] # Arguments for the command
    inheritEnv: true # Inherit parent environment?
    env: # Environment variables
      KEY: value
    prompt: "custom prompt" # Shell prompt (optional)
  stages: # Lifecycle hooks (optional)
    onInit: # Run before starting
      - script: echo "Starting..."
    postAttach: # Run after attaching
      - script: echo "Attached!"
```

## Field Reference

### Required fields

- **`metadata.name`**: A unique identifier for this profile. Use it with `sbsh -p <name>` to start a terminal.

### spec fields

#### `runTarget`

Where the terminal runs. Currently only `local` is supported.

#### `restartPolicy`

Controls what happens when the terminal exits:

- **`exit`**: Don't restart — terminal terminates (default for most use cases)
- **`restart-on-error`**: Only restart if the process exits with a non-zero code
- **`restart-unlimited`**: Always restart the terminal when it exits

**When to use each:**

- Use `exit` for SSH terminals, container shells, or when you want the terminal to stay dead after it exits
- Use `restart-on-error` for persistent services or when you want automatic recovery from crashes
- Use `restart-unlimited` for services that should always be running

#### `shell` section

Defines the command and environment for your terminal.

**`cwd`** (optional)

- Working directory where the terminal starts
- Use `~` for home directory (expands to `$HOME`)
- Use `$HOME/path` for paths relative to home
- Use absolute paths like `/tmp` for specific directories

**`cmd`** (required)

- Full path to the command or shell to execute
- Examples: `/bin/bash`, `/usr/bin/zsh`, `/usr/bin/ssh`, `/usr/bin/docker`

**`cmdArgs`** (optional)

- Array of arguments passed to `cmd`
- Example: `["--norc", "--noprofile"]` for bash
- Example: `["-t", "remote-host"]` for ssh
- Example: `["run", "--rm", "-ti", "ubuntu:latest"]` for docker

**`inheritEnv`** (optional, default: `true`)

- When `true`: terminal inherits all environment variables from the parent process
- When `false`: terminal starts with a minimal environment; only variables you specify in `env` are available
- Use `false` for reproducible, isolated environments
- Use `true` when you want access to your full environment (PATH, SSH_AUTH_SOCK, etc.)

**`env`** (optional)

- Key-value map of environment variables
- Values can reference other variables: `PATH: "$HOME/bin:$PATH"`
- Numbers should be quoted as strings: `HISTSIZE: "5000"`

**`prompt`** (optional)

- Custom shell prompt string
- Useful for identifying which profile you're using
- Supports shell escape sequences and sbsh variables:
  - `$SBSH_TERM_ID`: Current terminal ID
  - `$SBSH_TERM_PROFILE`: Current profile name
- **Quoting in YAML**: Complex prompts with escape sequences need careful quoting
  - Single quotes: `prompt: '"\[\e[1;31m\]...\[\e[0m\]" '`
  - Double quotes with escapes: `prompt: "\"[sbsh-$SBSH_TERM_ID] \\u@\\h:\\w$ \""`

#### `stages` section (optional)

Lifecycle hooks that run at specific points in the terminal lifecycle.

**`onInit`**

- List of commands that run **before** the main `cmd` starts
- Runs in sequence; if one fails, subsequent commands may not run
- Common uses:
  - Authenticating or configuring tools (`kubectl config use-context`)
  - Initializing environments (`terraform init`)
  - Setting up project state
- Each step is a script command:
  ```yaml
  onInit:
    - script: kubectl config use-context production
    - script: terraform workspace select prod
  ```

**`postAttach`**

- List of commands that run **after** you attach to the terminal
- Useful for showing status or context when you reconnect
- Common uses:
  - Showing current state (`kubectl get pods`)
  - Displaying project status
  - Running health checks
  ```yaml
  postAttach:
    - script: kubectl get pods
    - script: git status
  ```

## Example Profiles

This directory contains example profiles. The individual `.yaml` files are provided for reference, but sbsh only supports a single `profiles.yaml` file. All profiles must be combined into one file with `---` separators between documents.

### Profile Examples Index

| Profile             | Description                                                     | File                                                 | Section                                                           |
| ------------------- | --------------------------------------------------------------- | ---------------------------------------------------- | ----------------------------------------------------------------- |
| `default`           | Minimal bash terminal with clean environment                    | [`default.yaml`](./default.yaml)                     | [Section 1](#1-default---minimal-bash-terminal)                   |
| `zsh`               | Minimal zsh terminal with clean environment                     | [`zsh.yaml`](./zsh.yaml)                             | [Section 2](#2-zsh---minimal-zsh-terminal)                        |
| `k8s-default`       | Kubernetes development terminal with kubectl context setup      | [`k8s-default.yaml`](./k8s-default.yaml)             | [Section 3](#3-k8s-default---kubernetes-development-terminal)     |
| `terraform-prd`     | Terraform production workspace with environment variables       | [`terraform-prd.yaml`](./terraform-prd.yaml)         | [Section 4](#4-terraform-prd---terraform-production-workspace)    |
| `k8s-pod`           | Ephemeral Kubernetes pod shell for debugging                    | [`k8s-pod.yaml`](./k8s-pod.yaml)                     | [Section 5](#5-k8s-pod---ephemeral-kubernetes-pod-shell)          |
| `docker-container`  | Docker container shell for containerized workflows              | [`docker-container.yaml`](./docker-container.yaml)   | [Section 6](#6-docker-container---ephemeral-container-shell)      |
| `ssh-pk`            | SSH remote host connection with persistent sessions             | [`ssh-pk.yaml`](./ssh-pk.yaml)                       | [Section 7](#7-ssh-pk---ssh-to-remote-host)                       |
| `python-venv`       | Python virtual environment with automatic activation            | [`python-venv.yaml`](./python-venv.yaml)             | [Section 8](#8-python-venv---python-virtual-environment)          |
| `npm-dev`           | Node.js/npm development environment with dependency management  | [`npm-dev.yaml`](./npm-dev.yaml)                     | [Section 9](#9-npm-dev---nodejsnpm-development-environment)       |
| `go-dev`            | Go/Golang development environment with module support           | [`go-dev.yaml`](./go-dev.yaml)                       | [Section 10](#10-go-dev---gogolang-development)                   |
| `ci-test`           | CI/CD test environment for local testing that mirrors CI        | [`ci-test.yaml`](./ci-test.yaml)                     | [Section 11](#11-ci-test---cicd-test-environment)                 |
| `terraform-dev`     | Terraform development workspace with environment variables      | [`terraform-dev.yaml`](./terraform-dev.yaml)         | [Section 12](#12-terraform-dev---terraform-development-workspace) |
| `terraform-staging` | Terraform staging workspace with environment variables          | [`terraform-staging.yaml`](./terraform-staging.yaml) | [Section 13](#13-terraform-staging---terraform-staging-workspace) |
| `postgres`          | PostgreSQL database shell for database operations and debugging | [`postgres.yaml`](./postgres.yaml)                   | [Section 14](#14-postgres---postgresql-database-shell)            |
| `aws-cli`           | AWS CLI environment for infrastructure management               | [`aws-cli.yaml`](./aws-cli.yaml)                     | [Section 15](#15-aws-cli---aws-cli-environment)                   |
| `docker-compose`    | Docker Compose development environment for multi-service setups | [`docker-compose.yaml`](./docker-compose.yaml)       | [Section 16](#16-docker-compose---docker-compose-development)     |
| `redis`             | Redis CLI for database operations and debugging                 | [`redis.yaml`](./redis.yaml)                         | [Section 17](#17-redis---redis-cli)                               |
| `rust-dev`          | Rust development environment with toolchain setup               | [`rust-dev.yaml`](./rust-dev.yaml)                   | [Section 18](#18-rust-dev---rust-development-environment)         |
| `mysql`             | MySQL database shell for database operations and debugging      | [`mysql.yaml`](./mysql.yaml)                         | [Section 19](#19-mysql---mysql-database-shell)                    |
| `mongo`             | MongoDB database shell for database operations and debugging    | [`mongo.yaml`](./mongo.yaml)                         | [Section 20](#20-mongo---mongodb-database-shell)                  |

All profiles are also available in [`profiles.yaml`](./profiles.yaml) as a combined file ready to use.

**To use these profiles:**

```bash
# Copy the combined file
cp docs/profiles/profiles.yaml ~/.sbsh/profiles.yaml

# Or copy specific example files and combine them
cat docs/profiles/default.yaml docs/profiles/python-venv.yaml > ~/.sbsh/profiles.yaml
```

## Example Profiles Explained

### 1. `default` - Minimal bash terminal

See [`default.yaml`](./default.yaml) for the complete profile.

A clean bash terminal with minimal environment variables:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: default
spec:
  runTarget: local
  restartPolicy: restart-on-error
  shell:
    cwd: "~"
    cmd: /bin/bash
    cmdArgs: ["--norc", "--noprofile"] # Start with a clean bash
    inheritEnv: false # Don't inherit parent env
    env:
      LANG: en_US.UTF-8
      EDITOR: vim
      HISTSIZE: "5000"
    prompt: "\"[sbsh-$SBSH_TERM_ID] \\u@\\h:\\w$ \""
```

**Key points:**

- `inheritEnv: false` creates a minimal, reproducible environment
- `--norc --noprofile` prevents bash from loading your `.bashrc`
- Custom prompt shows the terminal ID so you know which sbsh terminal you're in

### 2. `zsh` - Minimal zsh terminal

See [`zsh.yaml`](./zsh.yaml) for the complete profile.

A clean zsh terminal with minimal environment variables, similar to the default bash profile but using zsh:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: zsh
spec:
  runTarget: local
  restartPolicy: restart-on-error
  shell:
    cwd: "~"
    cmd: /bin/zsh
    cmdArgs: ["-f"] # Start with a clean zsh (skip startup files)
    inheritEnv: false # Don't inherit parent env
    env:
      LANG: en_US.UTF-8
      EDITOR: vim
      HISTSIZE: "5000"
    prompt: '"[sbsh-$SBSH_TERM_ID] %n@%m:%~$ "'
```

**Key points:**

- `inheritEnv: false` creates a minimal, reproducible environment
- `-f` flag prevents zsh from loading startup files (`.zshrc`, `.zshenv`, etc.)
- Custom prompt uses zsh format (`%n@%m:%~$`) instead of bash format (`\u@\h:\w$`)
- `%n` is username, `%m` is hostname, `%~` is current directory in zsh prompt format

### 3. `k8s-default` - Kubernetes development terminal

See [`k8s-default.yaml`](./k8s-default.yaml) for the complete profile.

Sets up kubectl context and shows cluster info on attach.

**Key points:**

- `onInit` sets up the kubectl context before the shell starts
- `postAttach` shows cluster status when you reconnect
- Colored prompt distinguishes this profile from others

### 4. `terraform-prd` - Terraform production workspace

See [`terraform-prd.yaml`](./terraform-prd.yaml) for the complete profile.

Prepares a Terraform workspace and runs planning.

**Key points:**

- `restartPolicy: exit` prevents accidental restarts in production
- `onInit` prepares the Terraform workspace before you start working
- Environment variable `TF_VAR_environment` is available to Terraform

### 5. `k8s-pod` - Ephemeral Kubernetes pod shell

See [`k8s-pod.yaml`](./k8s-pod.yaml) for the complete profile.

Creates a temporary Kubernetes pod and drops you into a shell.

**Key points:**

- Uses `kubectl run` to create an ephemeral pod
- `--rm` removes the pod when it exits
- `-ti` allocates a pseudo-TTY and keeps STDIN open

### 6. `docker-container` - Ephemeral container shell

See [`docker-container.yaml`](./docker-container.yaml) for the complete profile.

Starts a temporary Docker container and drops you into a shell.

**Key points:**

- `cmd` is `docker`, not a shell
- `cmdArgs` contains all the docker arguments
- `--rm` removes the container when it exits
- `-ti` allocates a pseudo-TTY and keeps STDIN open

### 7. `ssh-pk` - SSH to remote host

See [`ssh-pk.yaml`](./ssh-pk.yaml) for the complete profile.

Connects to a remote host using SSH.

**Key points:**

- `cmd` is `ssh`
- `cmdArgs` contains SSH options and the host alias/name
- `-t` forces pseudo-TTY allocation (needed for interactive shells)

### 8. `python-venv` - Python virtual environment

See [`python-venv.yaml`](./python-venv.yaml) for the complete profile.

Sets up a Python development environment with virtual environment activation. Teams can share this profile to ensure everyone uses the same Python environment for local development.

**Key points:**

- `onInit` creates a virtual environment if it doesn't exist, then activates it
- Sets `PYTHONPATH` to include the project directory
- `postAttach` reactivates the venv and shows Python/pip versions when reconnecting
- Uses yellow-colored prompt to distinguish Python environments
- `inheritEnv: true` allows PATH and other tools to work correctly
- Teams can customize the `cwd` path to match their project location

**Customization:**

- Update `cwd` to point to your project directory
- Adjust `venv` directory name if your team uses a different convention (e.g., `.venv`, `env`)
- Add additional environment variables or setup steps as needed

### 9. `npm-dev` - Node.js/npm development environment

See [`npm-dev.yaml`](./npm-dev.yaml) for the complete profile.

Sets up a Node.js development environment with dependency installation. Teams can share this profile to ensure consistent Node.js environment setup across team members.

**Key points:**

- `onInit` checks Node/npm versions and installs dependencies if `node_modules` doesn't exist
- Sets `NODE_ENV=development` for proper development mode
- `postAttach` shows Node/npm versions and project name when reconnecting
- Uses cyan-colored prompt to distinguish Node.js environments
- `inheritEnv: true` allows PATH and npm/node binaries to work correctly
- Teams can customize the `cwd` path to match their project location

**Customization:**

- Update `cwd` to point to your project directory
- Modify `npm install` command if you use different package managers (yarn, pnpm) or need additional flags
- Add environment variables for API keys, database URLs, or other configuration
- Adjust lifecycle hooks to run tests, linting, or other setup steps

### 10. `go-dev` - Go/Golang development

See [`go-dev.yaml`](./go-dev.yaml) for the complete profile.

Sets up a Go development environment with module support. Teams can share this profile to ensure consistent Go environment setup across team members.

**Key points:**

- `onInit` checks Go version, shows GOROOT and GOPATH, and verifies Go modules if present
- Sets `GO111MODULE=on` to enable Go modules
- Sets `CGO_ENABLED=1` for CGO support (useful for many Go packages)
- `postAttach` shows Go version and module information when reconnecting
- Uses blue-colored prompt to distinguish Go environments
- `inheritEnv: true` allows PATH and Go tools to work correctly
- Teams can customize the `cwd` path to match their project location

**Customization:**

- Update `cwd` to point to your project directory
- Adjust `GO111MODULE` or `CGO_ENABLED` based on your project needs
- Add additional environment variables for Go-specific configuration
- Modify lifecycle hooks to run tests, builds, or other setup steps

### 11. `ci-test` - CI/CD test environment

See [`ci-test.yaml`](./ci-test.yaml) for the complete profile.

Sets up a local test environment that mirrors CI/CD pipelines. Use this profile to test locally with the same environment variables as your CI system.

**Key points:**

- Sets `CI=true` to indicate CI environment
- Sets `NODE_ENV=test` and `PYTHON_ENV=test` for test configurations
- `restartPolicy: exit` ensures clean test runs
- `onInit` confirms CI environment variables are set
- `postAttach` displays current environment status
- Uses purple-colored prompt to distinguish CI test environments
- `inheritEnv: true` allows access to system tools and PATH

**Customization:**

- Add additional CI-specific environment variables
- Customize test environment variables based on your CI/CD setup
- Add lifecycle hooks to run tests or setup test databases
- Adjust `cwd` to point to your project directory

### 12. `terraform-dev` - Terraform development workspace

See [`terraform-dev.yaml`](./terraform-dev.yaml) for the complete profile.

Prepares a Terraform development workspace with environment-specific configuration. Similar to `terraform-prd` but for development environments.

**Key points:**

- `restartPolicy: exit` prevents accidental restarts in development
- `onInit` prepares the Terraform workspace for development before you start working
- Environment variable `TF_VAR_environment: "dev"` is available to Terraform
- Uses blue-colored prompt (vs red for production) to distinguish environments
- Automatically runs `terraform workspace use dev` and `terraform init`

**Customization:**

- Update `cwd` to point to your Terraform project directory
- Add additional Terraform variables via `env` section
- Modify lifecycle hooks to run specific Terraform commands
- Adjust workspace name if your team uses different naming conventions

### 13. `terraform-staging` - Terraform staging workspace

See [`terraform-staging.yaml`](./terraform-staging.yaml) for the complete profile.

Prepares a Terraform staging workspace with environment-specific configuration. Similar to `terraform-prd` and `terraform-dev` but for staging environments.

**Key points:**

- `restartPolicy: exit` prevents accidental restarts in staging
- `onInit` prepares the Terraform workspace for staging before you start working
- Environment variable `TF_VAR_environment: "staging"` is available to Terraform
- Uses yellow-colored prompt (vs red for production, blue for dev) to distinguish environments
- Automatically runs `terraform workspace use staging` and `terraform init`

**Customization:**

- Update `cwd` to point to your Terraform project directory
- Add additional Terraform variables via `env` section
- Modify lifecycle hooks to run specific Terraform commands
- Adjust workspace name if your team uses different naming conventions

### 14. `postgres` - PostgreSQL database shell

See [`postgres.yaml`](./postgres.yaml) for the complete profile.

Connects to a PostgreSQL database using the `psql` CLI. Perfect for database operations, debugging, and administration with persistent terminal sessions.

**Key points:**

- `cmd` is `psql`, not a shell
- `cmdArgs` contains connection parameters: host, user, and database
- Uses environment variables for connection: `PGHOST`, `PGUSER`, `PGDATABASE`, `PGPORT`
- `restartPolicy: exit` ensures clean disconnection
- Password can be set via `PGPASSWORD` environment variable or entered interactively

**Customization:**

- Update environment variables to match your PostgreSQL connection details
- Add additional `psql` command-line arguments if needed
- Use connection string format: `psql "postgresql://user:password@host:port/database"`
- Set `PGPASSWORD` in environment for non-interactive password entry

### 15. `aws-cli` - AWS CLI environment

See [`aws-cli.yaml`](./aws-cli.yaml) for the complete profile.

Sets up an AWS CLI environment for infrastructure management. Perfect for managing AWS resources with consistent profile and region configuration.

**Key points:**

- `onInit` verifies AWS CLI installation and authenticates with AWS
- Sets AWS profile and region via environment variables: `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`
- `postAttach` shows current AWS identity and configuration when reconnecting
- Uses yellow-colored prompt showing AWS profile and region
- Verifies AWS credentials with `aws sts get-caller-identity`
- `inheritEnv: true` allows AWS CLI and other tools to work correctly

**Customization:**

- Update `AWS_PROFILE` to use a specific AWS profile
- Change `AWS_REGION` to your preferred default region
- Add additional AWS-specific environment variables
- Modify lifecycle hooks to verify specific AWS resources or run AWS commands

### 16. `docker-compose` - Docker Compose development

See [`docker-compose.yaml`](./docker-compose.yaml) for the complete profile.

Sets up a Docker Compose development environment for managing multi-service applications. Automatically starts services and shows status on attach.

**Key points:**

- `onInit` checks docker-compose version and starts services if `docker-compose.yml` exists
- `postAttach` shows service status and recent logs when reconnecting
- Sets `COMPOSE_PROJECT_NAME` to organize Docker Compose projects
- Uses cyan-colored prompt to distinguish Docker Compose environments
- `inheritEnv: true` allows Docker and docker-compose commands to work correctly
- Automatically runs `docker-compose up -d` to start services in detached mode

**Customization:**

- Update `cwd` to point to your project directory with `docker-compose.yml`
- Modify `COMPOSE_PROJECT_NAME` to match your project naming
- Add lifecycle hooks to run specific docker-compose commands
- Adjust service startup behavior (e.g., `docker-compose up` vs `docker-compose up -d`)

### 17. `redis` - Redis CLI

See [`redis.yaml`](./redis.yaml) for the complete profile.

Connects to a Redis instance using the `redis-cli` command. Perfect for debugging cache issues and performing Redis operations with persistent sessions.

**Key points:**

- `cmd` is `redis-cli`, not a shell
- `cmdArgs` contains connection parameters: host and port
- Uses environment variables for connection: `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`
- `restartPolicy: exit` ensures clean disconnection
- Password can be set via `REDIS_PASSWORD` environment variable or passed via `-a` flag

**Customization:**

- Update environment variables to match your Redis connection details
- Add password authentication: `-a "$REDIS_PASSWORD"` in `cmdArgs`
- Use connection string format: `redis-cli -u redis://password@host:port`
- Add additional Redis CLI options as needed

### 18. `rust-dev` - Rust development environment

See [`rust-dev.yaml`](./rust-dev.yaml) for the complete profile.

Sets up a Rust development environment with toolchain verification. Teams can share this profile to ensure consistent Rust environment setup across team members.

**Key points:**

- `onInit` checks Rust and Cargo versions, verifies Rust project if `Cargo.toml` exists
- Sets `RUST_BACKTRACE=1` for better error debugging
- `postAttach` shows Rust/Cargo versions and project information when reconnecting
- Uses red-colored prompt to distinguish Rust environments
- `inheritEnv: true` allows PATH and Rust tools to work correctly
- Teams can customize the `cwd` path to match their project location

**Customization:**

- Update `cwd` to point to your project directory
- Adjust `RUST_BACKTRACE` setting based on your debugging needs
- Add additional Rust-specific environment variables
- Modify lifecycle hooks to run tests, builds, or other setup steps

### 19. `mysql` - MySQL database shell

See [`mysql.yaml`](./mysql.yaml) for the complete profile.

Connects to a MySQL database using the `mysql` CLI. Perfect for database operations, debugging, and administration with persistent terminal sessions.

**Key points:**

- `cmd` is `mysql`, not a shell
- `cmdArgs` contains connection parameters: host, user, database, and password prompt
- Uses environment variables for connection: `MYSQL_HOST`, `MYSQL_USER`, `MYSQL_DATABASE`, `MYSQL_PASSWORD`
- `restartPolicy: exit` ensures clean disconnection
- Password can be set via `MYSQL_PWD` environment variable or entered interactively with `-p` flag

**Customization:**

- Update environment variables to match your MySQL connection details
- Use `MYSQL_PWD` environment variable for non-interactive password entry (less secure)
- Add additional MySQL command-line arguments if needed
- Use connection string format: `mysql -h host -u user -p password database`

### 20. `mongo` - MongoDB database shell

See [`mongo.yaml`](./mongo.yaml) for the complete profile.

Connects to a MongoDB database using the `mongosh` CLI (MongoDB Shell). Perfect for database operations, debugging, and administration with persistent terminal sessions.

**Key points:**

- `cmd` is `mongosh`, not a shell
- `cmdArgs` contains connection URI: `mongodb://localhost:27017` by default
- Uses environment variables for connection: `MONGO_URI`, `MONGO_HOST`, `MONGO_PORT`, `MONGO_DATABASE`, `MONGO_USER`, `MONGO_PASSWORD`
- `restartPolicy: exit` ensures clean disconnection
- Supports connection string format: `mongodb://user:password@host:port/database`

**Customization:**

- Update `MONGO_URI` to match your MongoDB connection string
- Use individual environment variables (`MONGO_HOST`, `MONGO_PORT`, etc.) to build connection string
- Add authentication: `mongodb://username:password@host:port/database`
- Add additional mongosh command-line arguments if needed
- Use connection string with authentication: Update `MONGO_URI` to include credentials

## Creating Your Own Profile

### Step-by-step guide

1. **Start with a copy**: Copy an existing profile that's close to what you need
2. **Change the name**: Update `metadata.name` to something meaningful
3. **Customize the command**: Set `cmd` and `cmdArgs` for what you want to run
4. **Set the working directory**: Use `cwd` to start in the right place
5. **Configure environment**: Add `env` variables or set `inheritEnv`
6. **Add lifecycle hooks**: Use `onInit` and `postAttach` if needed
7. **Test it**: Run `sbsh -p <your-profile-name>` and verify everything works

### Best practices

1. **Use descriptive names**: `my-project-dev` is better than `test1`
2. **Prefer `cmdArgs` arrays**: Keep `cmd` clean and use `cmdArgs` for arguments
3. **Use `inheritEnv: false` for isolation**: Better for reproducible environments
4. **Use `inheritEnv: true` for convenience**: When you need full PATH and other tools
5. **Quote numbers in env**: `HISTSIZE: "5000"` not `HISTSIZE: 5000`
6. **Use `~` for home directory**: It's expanded at terminal start
7. **Use `restartPolicy: exit` for containers/SSH**: These should exit cleanly
8. **Use `restartPolicy: restart-on-error` for services**: When you want resilience

## Common Issues and Solutions

### Profile not found

**Problem**: `sbsh` says "profile not found"

**Solutions:**

- Check the profile name matches exactly (case-sensitive)
- Verify `~/.sbsh/profiles.yaml` exists
- Run `sb get profiles` to see available profiles
- Check if you're using a custom profiles file path

### Environment variables not working

**Problem**: Environment variables aren't available in the terminal

**Solutions:**

- If `inheritEnv: false`, make sure variables are in the `env` map
- Use `$HOME` instead of `~` in env values
- Quote variable values that contain spaces or special characters
- Numbers must be strings: `"5000"` not `5000`

### Prompt not showing colors

**Problem**: Shell prompt lacks colors or escape sequences

**Solutions:**

- Quote prompts carefully in YAML
- Use double quotes around the prompt string: `prompt: '"\[\e[1;31m\]...\""`
- Test with simpler prompts first, then add complexity
- Remember to escape backslashes: `\\u` not `\u`

### Stages not running

**Problem**: Commands in `onInit` or `postAttach` don't execute

**Solutions:**

- Verify YAML syntax is correct (indentation matters!)
- Check that `script:` is indented under each stage item
- Ensure commands are available in the environment
- Use absolute paths for commands if PATH isn't set

### Tilde (`~`) not expanding

**Problem**: `~` appears literally instead of expanding to home directory

**Solutions:**

- Tilde expands in `cwd` but not in `env` values
- Use `$HOME` in environment variables: `PATH: "$HOME/bin:$PATH"`
- Use absolute paths if expansion is unreliable

## Testing Your Profiles

1. **Validate syntax**: Use a YAML validator or check with `yaml` tools
2. **List profiles**: Run `sb get profiles` to see if your profile appears
3. **Test the profile**: Run `sbsh -p <profile-name>` to start a terminal
4. **Check environment**: Run `env` to verify variables are set correctly
5. **Test stages**: Detach and reattach to verify `postAttach` runs
6. **Test restart policy**: Exit the terminal and see if it restarts as expected

## Viewing Available Profiles

List all profiles in your profiles file:

```bash
sb get profiles
```

This shows:

- Profile name
- Run target
- Number of environment variables
- Command that will be executed

## Custom Profile File Location

By default, sbsh uses `~/.sbsh/profiles.yaml`. To use a different location:

**Using environment variable:**

```bash
export SBSH_PROFILES_FILE=/path/to/custom/profiles.yaml
sbsh -p my-profile
```

**Using command flag:**

```bash
sbsh --profiles-file /path/to/custom/profiles.yaml -p my-profile
```

## Profile File Organization

sbsh loads all profiles from a single `profiles.yaml` file. Multiple profiles are defined in the same file, separated by `---` (YAML document separator).

Example profiles are provided as individual files in this directory for reference and documentation purposes. To use them, you must combine them into a single `profiles.yaml` file:

```bash
# Copy the pre-combined file (recommended)
cp docs/profiles/profiles.yaml ~/.sbsh/profiles.yaml

# Or combine specific example files
cat docs/profiles/default.yaml docs/profiles/python-venv.yaml > ~/.sbsh/profiles.yaml
```

**Important**: sbsh only reads from a single `profiles.yaml` file. The individual `.yaml` files in `docs/profiles/` are examples for reference only.

## Contributing Examples

Found a useful profile pattern? We'd love to see it! Consider opening a pull request with:

- Your profile as a new file in `docs/profiles/` (e.g., `my-profile.yaml`) - provided as a reference example
- Add the profile to `docs/profiles/profiles.yaml` (the combined file) with `---` separator
- Update this README to document the new profile
- A brief description of the use case
- Any special requirements or setup instructions

---

Happy profiling! 🚀
