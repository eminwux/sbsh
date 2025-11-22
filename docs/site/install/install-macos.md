# Installing sbsh on macOS

Install sbsh on macOS systems.

## Quick Install

```bash
# Set your architecture (Apple Silicon uses arm64, Intel uses amd64)
export ARCH=arm64      # For Apple Silicon (M1/M2/M3)
# export ARCH=amd64    # For Intel Macs

# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-darwin-${ARCH} && \
chmod +x sbsh && \
sudo install -m 0755 sbsh /usr/local/bin/sbsh && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

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

Add to `~/.bashrc` or `~/.bash_profile`:

```bash
cat >> ~/.bash_profile <<EOF
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
EOF
```

Reload your shell:

```bash
source ~/.bash_profile
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

### Homebrew (Coming Soon)

Homebrew installation is planned for future releases.

### Manual Installation

Download and install manually:

```bash
# Download (Apple Silicon)
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-darwin-arm64

# Or for Intel Macs
# curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-darwin-amd64

# Make executable
chmod +x sbsh

# Install
sudo mv sbsh /usr/local/bin/sbsh
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

### User-local Installation

Install to `~/bin` (add to PATH):

```bash
mkdir -p ~/bin
curl -L -o ~/bin/sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-darwin-arm64
chmod +x ~/bin/sbsh
ln -f ~/bin/sbsh ~/bin/sb

# Add to PATH in ~/.zshrc
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## Troubleshooting

### Gatekeeper Warnings

macOS may show a security warning. To allow the binary:

1. Open System Settings → Privacy & Security
2. Click "Open Anyway" if prompted
3. Or remove the quarantine attribute:

```bash
xattr -d com.apple.quarantine sbsh
```

### Architecture Mismatch

Verify your Mac's architecture:

```bash
# Check architecture
uname -m
# arm64 for Apple Silicon, x86_64 for Intel
```

Use the correct binary:

- Apple Silicon (M1/M2/M3): `sbsh-darwin-arm64`
- Intel Macs: `sbsh-darwin-amd64`

### Command Not Found

If `sbsh` is not found after installation:

1. Verify the binary is in your PATH: `which sbsh`
2. Check `/usr/local/bin` is in PATH: `echo $PATH`
3. Reload your shell: `source ~/.zshrc` or `source ~/.bash_profile`

## Next Steps

- [Getting Started](../getting-started.md) - Your first terminal session
- [Prerequisites](prerequisites.md) - System requirements
