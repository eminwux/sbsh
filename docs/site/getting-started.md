# Getting Started

Get sbsh up and running in minutes. This guide covers installation, basic configuration, and your first terminal session.

## Installation

sbsh is available for Linux, macOS, and FreeBSD. See the [Installation](install/prerequisites.md) section for platform-specific instructions.

### Quick Install

```bash
# Set your platform (defaults shown)
export OS=linux        # Options: linux, darwin, freebsd
export ARCH=amd64      # Options: amd64, arm64

# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-${OS}-${ARCH} && \
chmod +x sbsh && \
sudo install -m 0755 sbsh /usr/local/bin/sbsh && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

### Autocomplete Setup

Enable command completion for bash:

```bash
cat >> ~/.bashrc <<EOF
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
EOF
```

For zsh, add to `~/.zshrc`:

```bash
cat >> ~/.zshrc <<EOF
source <(sbsh autocomplete zsh)
source <(sb autocomplete zsh)
EOF
```

## Your First Terminal

Start a terminal with the default profile:

```bash
$ sbsh
To detach, press ^] twice
[sbsh-35af93de] user@host:~/projects$
```

You're now in an sbsh terminal! The terminal ID (`35af93de`) is shown in the prompt.

### Detach from Terminal

Press `Ctrl-]` twice to detach. The terminal keeps running:

```bash
[sbsh-35af93de] user@host:~/projects$ # Press Ctrl-] twice
Detached
$  # Back to your shell, but terminal is still running
```

### List Active Terminals

```bash
$ sb get terminals
ID        NAME           PROFILE  CMD                           STATUS
bbb4b457  quick_samwise  default  /bin/bash --norc --noprofile  Ready
```

### Reattach to a Terminal

```bash
$ sb attach quick_samwise
[sbsh-bbb4b457] user@host:~/projects$ # Back where you left off!
```

## Using Profiles

Profiles define terminal environments declaratively. Start a terminal with a profile:

```bash
$ sbsh -p k8s-default
# Automatically configures kubectl context, shows cluster status
```

List available profiles:

```bash
$ sb get profiles
NAME              TARGET  ENVVARS  CMD
k8s-default       local   4 vars   /bin/bash
terraform-prd     local   2 vars   /bin/bash
docker-container  local   0 vars   /usr/bin/docker run ...
```

See the [Profiles Guide](guides/profiles.md) for creating your own profiles.

## Next Steps

- **[Concepts](concepts/terminals.md)** - Learn about terminals, profiles, and supervisors
- **[Create Your First Profile](tutorials/create-your-first-profile.md)** - Build a custom profile
- **[CLI Reference](cli/commands.md)** - Explore all available commands
- **[FAQ](faq.md)** - Common questions and answers
