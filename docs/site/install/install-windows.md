# Installing sbsh on Windows

Windows support for sbsh is planned but not yet available. Use one of these alternatives:

## Option 1: Windows Subsystem for Linux (WSL)

Install sbsh in WSL and use it from Windows:

### Install WSL

```powershell
# Install WSL (if not already installed)
wsl --install
```

### Install sbsh in WSL

Follow the [Linux installation instructions](install-linux.md) inside your WSL environment.

### Usage

Access sbsh from Windows:

```powershell
# From PowerShell or CMD
wsl sbsh
wsl sb get terminals
```

## Option 2: Docker

Run sbsh in a Docker container on Windows:

### Prerequisites

- [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop)

### Run sbsh Container

```powershell
# Run sbsh interactively
docker run -it --rm `
  -v ${env:USERPROFILE}\.sbsh:/root/.sbsh `
  docker.io/eminwux/sbsh:v0.5.0-linux-amd64 `
  sbsh
```

See the [Container Usage Guide](../guides/container.md) for detailed Docker instructions.

## Option 3: Virtual Machine

Run sbsh in a Linux virtual machine:

1. Install a Linux distribution (Ubuntu, Debian, etc.) in a VM
2. Follow the [Linux installation instructions](install-linux.md)
3. Access via SSH or VM console

## Native Windows Support

Native Windows support is planned for future releases. This will include:

- Native Windows binary
- Windows-specific path handling
- Integration with Windows Terminal
- PowerShell profile support

## Next Steps

- [Getting Started](../getting-started.md) - Your first terminal session
- [Container Usage](../guides/container.md) - Using sbsh in Docker
