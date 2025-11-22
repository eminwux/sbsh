# Create Your First Profile

This tutorial walks you through creating your first sbsh profile step by step.

## Prerequisites

- sbsh installed (see [Installation](../install/prerequisites.md))
- Basic understanding of YAML syntax

## Step 1: Create Profiles Directory

Create the profiles directory if it doesn't exist:

```bash
mkdir -p ~/.sbsh
```

## Step 2: Create a Minimal Profile

Create your first profile in `~/.sbsh/profiles.yaml`:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-first-profile
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cmd: /bin/bash
```

This is the simplest profile possible. It will:

- Start a bash terminal
- Use default settings
- Exit when the terminal exits

## Step 3: Test Your Profile

Start a terminal with your profile:

```bash
sbsh -p my-first-profile
```

You should see a terminal prompt. Type `exit` to close it.

## Step 4: Add Environment Variables

Enhance your profile with environment variables:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-first-profile
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cmd: /bin/bash
    env:
      MY_VAR: "hello world"
      PROJECT_DIR: "$HOME/projects"
```

## Step 5: Set Working Directory

Add a working directory:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-first-profile
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cwd: "~"
    cmd: /bin/bash
    env:
      MY_VAR: "hello world"
```

## Step 6: Add Lifecycle Hooks

Add initialization commands:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-first-profile
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cwd: "~"
    cmd: /bin/bash
    env:
      MY_VAR: "hello world"
  stages:
    onInit:
      - script: echo "Terminal starting..."
    postAttach:
      - script: echo "Welcome back!"
```

## Step 7: Customize the Prompt

Add a custom prompt to identify your profile:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: my-first-profile
spec:
  runTarget: local
  restartPolicy: exit
  shell:
    cwd: "~"
    cmd: /bin/bash
    env:
      MY_VAR: "hello world"
    prompt: '"\[\e[1;34m\]my-profile \[\e[0m\]\u@\h:\w\$ "'
  stages:
    onInit:
      - script: echo "Terminal starting..."
    postAttach:
      - script: echo "Welcome back!"
```

## Step 8: Verify Your Profile

List your profiles to verify:

```bash
sb get profiles
```

You should see `my-first-profile` in the list.

## Next Steps

- [Profiles Guide](../guides/profiles.md) - Complete profile reference
- [Share a Terminal](share-a-terminal.md) - Share terminals with your team
- [Environment Isolation](../concepts/environment-isolation.md) - Control environment inheritance
