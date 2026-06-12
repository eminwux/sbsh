# Installing sbsh on Linux

Install sbsh on Linux systems (Ubuntu, Debian, RHEL, CentOS, etc.).

## Quick Install

One-liner — auto-detects arch, resolves the latest release, and installs `sbsh` plus the `sb` hardlink:

```bash
curl -fsSL https://sbsh.io/install.sh | bash
```

Override defaults via env vars: `SBSH_VERSION=vX.Y.Z` (pin a tag), `SBSH_INSTALL_PREFIX=/path/bin` (default `/usr/local/bin`), `SBSH_REPO=owner/repo` (forks), `SBSH_SKIP_CHECKSUM=1`. Pass `--check` to run prerequisite checks only (`curl -fsSL https://sbsh.io/install.sh | bash -s -- --check`), or `--help` for usage.

For air-gapped or pinned-release installs, see [Manual Installation](#manual-installation) below.

## Verify Installation

```bash
# Check version
sbsh --version
sb --version

# Test basic functionality
sbsh --help
sb --help
```

## Autocomplete Setup

### Bash

Add to `~/.bashrc`:

```bash
cat >> ~/.bashrc <<EOF
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
EOF
```

Reload your shell:

```bash
source ~/.bashrc
```

### Zsh

Add to `~/.zshrc`:

```bash
cat >> ~/.zshrc <<EOF
source <(sbsh autocomplete zsh)
source <(sb autocomplete zsh)
EOF
```

Reload your shell:

```bash
source ~/.zshrc
```

## Alternative Installation Methods

### Manual Installation

Download and install manually:

```bash
# Download
wget https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-linux-amd64

# Make executable
chmod +x sbsh-linux-amd64

# Install
sudo mv sbsh-linux-amd64 /usr/local/bin/sbsh
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

### User-local Installation

Install to `~/bin` (add to PATH):

```bash
mkdir -p ~/bin
curl -L -o ~/bin/sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-linux-amd64
chmod +x ~/bin/sbsh
ln -f ~/bin/sbsh ~/bin/sb

# Add to PATH in ~/.bashrc or ~/.zshrc
export PATH="$HOME/bin:$PATH"
```

## Troubleshooting

### Permission Denied

If you get permission errors, ensure the binary is executable:

```bash
chmod +x sbsh
```

### Command Not Found

If `sbsh` is not found after installation:

1. Verify the binary is in your PATH: `which sbsh`
2. Check `/usr/local/bin` is in PATH: `echo $PATH`
3. Reload your shell: `source ~/.bashrc` or `source ~/.zshrc`

### PTY Issues

If you encounter PTY-related errors:

```bash
# Check PTY support
test -c /dev/ptmx && echo "PTY available"

# Check permissions
ls -l /dev/ptmx
```

## Next Steps

- [Getting Started](../getting-started.md) - Your first terminal session
- [Prerequisites](prerequisites.md) - System requirements
