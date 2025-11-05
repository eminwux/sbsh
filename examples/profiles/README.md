# Terminal Profiles for sbsh

Profiles define how sbsh starts and manages terminal terminals. They let you customize the command, environment, working directory, and lifecycle hooks for your terminals.

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

2. Copy the example profiles file to get started:

   ```bash
   cp examples/profiles/profiles.yaml ~/.sbsh/profiles.yaml
   ```

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

## Example Profiles Explained

### 1. `default` - Minimal bash terminal

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

### 2. `k8s-default` - Kubernetes development terminal

Sets up kubectl context and shows cluster info on attach:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: k8s-default
spec:
  runTarget: local
  restartPolicy: restart-on-error
  shell:
    cwd: "~/projects"
    cmd: /bin/bash
    env:
      KUBECONF: "$HOME/.kube/config"
      KUBE_CONTEXT: default
      KUBE_NAMESPACE: default
    prompt: '"\[\e[1;31m\]sbsh($SBSH_TERM_PROFILE/$SBSH_TERM_ID) \[\e[1;32m\]\u@\h\[\e[0m\]:\w\$ "'
  stages:
    onInit:
      - script: kubectl config use-context $KUBE_CONTEXT
      - script: kubectl config get-contexts
    postAttach:
      - script: kubectl get ns
      - script: kubectl -n $KUBE_NAMESPACE get pods
```

**Key points:**

- `onInit` sets up the kubectl context before the shell starts
- `postAttach` shows cluster status when you reconnect
- Colored prompt distinguishes this profile from others

### 3. `terraform-prd` - Terraform production workspace

Prepares a Terraform workspace and runs planning:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: terraform-prd
spec:
  runTarget: local
  restartPolicy: exit # Don't auto-restart in production
  shell:
    cwd: "~/project/terraform"
    cmd: /bin/bash
    env:
      TF_VAR_environment: "prd" # Sets Terraform variable
  stages:
    onInit:
      - script: terraform workspace use $TF_VAR_environment
      - script: terraform init
      - script: terraform plan -out=tfplan
```

**Key points:**

- `restartPolicy: exit` prevents accidental restarts in production
- `onInit` prepares the Terraform workspace before you start working
- Environment variable `TF_VAR_environment` is available to Terraform

### 4. `docker-container` - Ephemeral container shell

Starts a temporary Docker container and drops you into a shell:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: docker-container
spec:
  runTarget: local
  restartPolicy: exit # Container exits when shell exits
  shell:
    cmd: /usr/bin/docker
    cmdArgs: ["run", "--rm", "-ti", "debian:stable-slim", "/bin/bash"]
```

**Key points:**

- `cmd` is `docker`, not a shell
- `cmdArgs` contains all the docker arguments
- `--rm` removes the container when it exits
- `-ti` allocates a pseudo-TTY and keeps STDIN open

### 5. `ssh-pk` - SSH to remote host

Connects to a remote host using SSH:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: ssh-pk
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cmd: /usr/bin/ssh
    cmdArgs: ["-t", "pk"] # "pk" is an SSH host alias
```

**Key points:**

- `cmd` is `ssh`
- `cmdArgs` contains SSH options and the host alias/name
- `-t` forces pseudo-TTY allocation (needed for interactive shells)

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

## Contributing Examples

Found a useful profile pattern? We'd love to see it! Consider opening a pull request with:

- Your profile in `examples/profiles/profiles.yaml`
- A brief description of the use case
- Any special requirements or setup instructions

---

Happy profiling! 🚀
