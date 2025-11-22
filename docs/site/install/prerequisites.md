# Prerequisites

System requirements and dependencies for installing sbsh.

## System Requirements

### Supported Operating Systems

- **Linux**: All major distributions (Ubuntu, Debian, RHEL, CentOS, etc.)
- **macOS**: macOS 10.14 (Mojave) or later
- **FreeBSD**: FreeBSD 12.0 or later

### Supported Architectures

- **amd64** (x86_64): Intel and AMD processors
- **arm64**: ARM64 processors (Apple Silicon, AWS Graviton, etc.)

### System Dependencies

sbsh requires minimal system dependencies:

- **procps**: Process management utilities (usually pre-installed on Linux)
- **Unix domain sockets**: For inter-process communication
- **PTY support**: For terminal emulation

### Permissions

sbsh requires:

- Write access to `~/.sbsh/` directory (created automatically)
- Ability to create Unix domain sockets
- PTY device access (usually available by default)

## Verification

Check if your system meets the requirements:

```bash
# Check architecture
uname -m

# Check for procps (Linux)
which ps

# Check PTY support
test -c /dev/ptmx && echo "PTY support available"
```

## Next Steps

- [Linux Installation](install-linux.md)
- [macOS Installation](install-macos.md)
- [Windows Installation](install-windows.md)
