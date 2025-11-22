# Container Usage with sbsh

sbsh provides official Docker images for running persistent terminals in containerized environments. Use the pre-built images to quickly deploy sbsh in Docker, Kubernetes, or any container orchestration platform.

## Key Benefits for Container Usage

- **Pre-built Images**: Ready-to-use Docker images for multiple architectures
- **Persistence**: Terminal sessions survive container restarts when volumes are properly configured
- **Isolation**: Run sbsh in isolated environments without affecting the host system
- **Portability**: Same image works across different container platforms
- **CI/CD Integration**: Use containers in CI/CD pipelines for reproducible environments

## Pulling sbsh Images

sbsh images are available on Docker Hub at `docker.io/eminwux/sbsh`. Images are tagged with version and architecture:

### AMD64 (x86_64)

```bash
docker pull docker.io/eminwux/sbsh:v0.6.0-linux-amd64
```

### ARM64

```bash
docker pull docker.io/eminwux/sbsh:v0.6.0-linux-arm64
```

### Using Docker Hub Short Syntax

```bash
# AMD64
docker pull eminwux/sbsh:v0.6.0-linux-amd64

# ARM64
docker pull eminwux/sbsh:v0.6.0-linux-arm64
```

## Image Architecture

The sbsh Docker image is built using a multi-stage build process:

### Build Stage

- **Base**: `golang:1.25-bookworm` (builder stage)
- **Build Process**: Compiles sbsh binary for the target architecture
- **Build Args**: `ARCH` (amd64/arm64) and `OS` (linux)

### Runtime Stage

- **Base**: `debian:bookworm-slim` (minimal Debian image)
- **Dependencies**: `procps` package (for process management utilities)
- **Binary**: `/bin/sbsh` and `/bin/sb` (hard-linked)
- **Default CMD**: `["/bin/sbsh", "terminal"]` (runs terminal only by default; can be overridden to use `CMD ["/bin/sbsh"]` for supervisor+terminal)

### Image Structure

```
/bin/sbsh          # Main sbsh binary
/bin/sb            # Hard link to sbsh (client tool)
```

## Choosing the Right CMD

The sbsh Docker image supports two different CMD options depending on your use case:

### `CMD ["/bin/sbsh"]` - Supervisor + Terminal (Interactive)

Runs a **supervisor attached to a terminal**. This is the default behavior when using `sbsh` interactively:

- **Supervisor**: Manages the terminal and stays attached
- **Terminal**: Interactive shell session
- **Use case**: Interactive development, debugging, login shells
- **Behavior**: You interact directly with the terminal; supervisor stays attached

**Example Dockerfile:**

```dockerfile
FROM docker.io/eminwux/sbsh:v0.6.0-linux-amd64
CMD ["/bin/sbsh"]
```

**When to use:**

- Interactive containers where you need to attach and detach
- Login shell scenarios
- Development environments where you want direct terminal access
- When you need the supervisor to manage the terminal lifecycle

### `CMD ["/bin/sbsh", "terminal"]` - Terminal Only

Runs **just a terminal** without a supervisor. The supervisor is launched externally when you attach:

- **Supervisor**: Not running initially; launched when you execute `sb attach <name>` from outside
- **Terminal**: Independent terminal session that runs on its own
- **Use case**: Background services, automation, CI/CD pipelines
- **Behavior**: Terminal runs independently; supervisor is created when you attach via `sb attach`
- **Important**: Always use `--name <name>` with `sbsh terminal` to identify the terminal for later attachment

**Example Dockerfile:**

```dockerfile
FROM docker.io/eminwux/sbsh:v0.6.0-linux-amd64
CMD ["/bin/sbsh", "terminal", "--name", "my-terminal"]
```

**When to use:**

- Background terminals that need to persist
- Automation and CI/CD pipelines
- Services that should run detached
- When terminals need to survive supervisor restarts
- When you want to attach to the terminal later from outside the container

**Attaching to the terminal:**

```bash
# From outside the container (if you have sb installed):
sb attach my-terminal

# Or from within the container:
docker exec -it <container> sb attach my-terminal
```

### Comparison

| Feature           | `CMD ["/bin/sbsh"]`     | `CMD ["/bin/sbsh", "terminal"]`                 |
| ----------------- | ----------------------- | ----------------------------------------------- |
| **Supervisor**    | Launched and attached   | Not running initially; launched when you attach |
| **Terminal**      | Managed by supervisor   | Independent, runs on its own                    |
| **Use case**      | Interactive development | Background services                             |
| **Detach**        | Press `Ctrl-]` twice    | No supervisor attached initially                |
| **Attach**        | Already attached        | Launch supervisor with `sb attach <name>`       |
| **Name required** | Optional                | **Required** (use `--name`)                     |

## Basic Usage

### Running sbsh Interactively

Run sbsh with an attached supervisor and terminal:

```bash
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh
```

**Key Points:**

- `-v ~/.sbsh:/root/.sbsh`: Mounts your local sbsh directory to persist terminals and metadata
- `-it`: Interactive terminal mode
- `--rm`: Remove container when it exits (optional)
- Default user is `root`; adjust paths if using a different user

### Running a Detached Terminal

Launch a terminal that runs independently (no supervisor initially). **Important**: Always use `--name` to identify the terminal:

```bash
docker run -d \
  --name my-sbsh-terminal \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh terminal --name my-terminal
```

The terminal runs independently. A supervisor is not running initially - it will be launched when you attach via `sb attach`.

### Attaching to a Running Terminal

Use the `sb` client to attach to a terminal running in a container:

```bash
docker exec -it my-sbsh-terminal sb attach my-terminal
```

Or from the host, if you have `sb` installed locally:

```bash
sb attach my-terminal
```

This works because the `~/.sbsh` volume is shared between the container and host.

## Volume Requirements

sbsh requires persistent storage for terminal metadata, sockets, and logs. Mount the `~/.sbsh` directory as a volume:

### Volume Mount

```bash
-v ~/.sbsh:/root/.sbsh
```

### What Gets Stored

The `~/.sbsh` directory contains:

- **`~/.sbsh/run/terminals/`**: Terminal metadata and socket files
- **`~/.sbsh/run/supervisors/`**: Supervisor metadata
- **`~/.sbsh/profiles.yaml`**: Profile definitions (optional)

### Using Named Volumes

For better Docker management, use named volumes:

```bash
docker volume create sbsh-data

docker run -it --rm \
  -v sbsh-data:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh
```

### Using Bind Mounts

For direct access to files from the host:

```bash
docker run -it --rm \
  -v /home/user/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh
```

## Running sbsh Commands

### Interactive Mode (`sbsh`)

Launch a supervisor with an attached terminal:

```bash
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh -p terraform-prd
```

### Detached Terminal (`sbsh terminal`)

Create a terminal that runs independently:

```bash
docker run -d \
  --name terraform-terminal \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh terminal --name terraform-prd -p terraform-prd
```

### Client Commands (`sb`)

List terminals, attach, or manage sessions:

```bash
# List terminals
docker run --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sb get terminals

# Attach to a terminal
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sb attach my-terminal

# List profiles
docker run --rm \
  -v ~/.sbsh:/root/.sbsh \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sb get profiles
```

## Using sbsh as a Base Image

You can use the sbsh image as a base for custom Dockerfiles:

### Example 1: Interactive Development Environment (Supervisor + Terminal)

Use `CMD ["/bin/sbsh"]` for interactive development where you want the supervisor attached:

```dockerfile
FROM docker.io/eminwux/sbsh:v0.6.0-linux-amd64

# Install additional tools
RUN apt-get update && apt-get install -y \
    git \
    vim \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy custom profiles
COPY profiles.yaml /root/.sbsh/profiles.yaml

# Set working directory
WORKDIR /workspace

# Run supervisor + terminal (interactive)
CMD ["/bin/sbsh"]
```

**Building and Running:**

```bash
docker build -t my-sbsh-env .
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  -v $(pwd):/workspace \
  my-sbsh-env
```

This runs an interactive terminal with an attached supervisor. Press `Ctrl-]` twice to detach.

### Example 2: Background Terminal Service (Terminal Only)

Use `CMD ["/bin/sbsh", "terminal"]` for background terminals that run independently. **Important**: Always specify `--name` to identify the terminal for later attachment:

```dockerfile
FROM docker.io/eminwux/sbsh:v0.6.0-linux-amd64

# Install additional tools
RUN apt-get update && apt-get install -y \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy custom profiles
COPY profiles.yaml /root/.sbsh/profiles.yaml

# Set working directory
WORKDIR /workspace

# Run terminal only (no supervisor initially)
# The supervisor will be launched when you attach via sb attach
CMD ["/bin/sbsh", "terminal", "--name", "background-terminal", "-p", "default"]
```

**Building and Running:**

```bash
docker build -t my-sbsh-background .
docker run -d \
  --name my-terminal-service \
  -v ~/.sbsh:/root/.sbsh \
  my-sbsh-background

# Attach later - this launches a supervisor to connect to the terminal:
docker exec -it my-terminal-service sb attach background-terminal
# Or from host (if sb is installed):
sb attach background-terminal
```

This runs a terminal that continues independently. The supervisor is not running initially - it's launched when you execute `sb attach background-terminal`, which creates a supervisor to connect to the existing terminal.

## Docker Compose Example

Use Docker Compose for easier management:

```yaml
version: "3.8"

services:
  sbsh:
    image: docker.io/eminwux/sbsh:v0.6.0-linux-amd64
    container_name: sbsh
    volumes:
      - ~/.sbsh:/root/.sbsh
      - ./workspace:/workspace
    stdin_open: true
    tty: true
    command: sbsh -p default
    environment:
      - TERM=xterm-256color
```

Run with:

```bash
docker-compose up
```

## Multi-Architecture Support

sbsh images are available for multiple architectures:

- **linux/amd64**: x86_64 processors (Intel, AMD)
- **linux/arm64**: ARM64 processors (Apple Silicon, AWS Graviton, etc.)

### Platform-Specific Pulls

Docker automatically selects the correct image for your platform:

```bash
docker pull eminwux/sbsh:v0.6.0-linux-amd64
docker pull eminwux/sbsh:v0.6.0-linux-arm64
```

### Using Docker Buildx

For multi-architecture builds, use Docker Buildx:

```bash
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 \
  -t eminwux/sbsh:v0.6.0 \
  --push .
```

## Docker-in-Docker (DinD)

If you need to run Docker commands inside sbsh containers, you can use Docker-in-Docker:

### Using Docker Socket

```bash
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  -v /var/run/docker.sock:/var/run/docker.sock \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh
```

Then install Docker client inside the container or use a profile that includes it.

### Using DinD Container

```bash
docker run -it --rm \
  -v ~/.sbsh:/root/.sbsh \
  --privileged \
  docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
  sbsh
```

**Security Note**: Using `--privileged` or mounting the Docker socket grants extensive permissions. Use with caution in production environments.

## Kubernetes Integration

### Deployment Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sbsh
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sbsh
  template:
    metadata:
      labels:
        app: sbsh
    spec:
      containers:
        - name: sbsh
          image: docker.io/eminwux/sbsh:v0.6.0-linux-amd64
          command: ["/bin/sbsh", "terminal", "--name", "k8s-terminal"]
          volumeMounts:
            - name: sbsh-data
              mountPath: /root/.sbsh
          stdin: true
          tty: true
      volumes:
        - name: sbsh-data
          persistentVolumeClaim:
            claimName: sbsh-pvc
```

### Persistent Volume Claim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: sbsh-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

## Troubleshooting

### Permission Denied Errors

**Problem**: Cannot write to `~/.sbsh` directory

**Solutions**:

- Check volume mount permissions
- Ensure directory exists: `mkdir -p ~/.sbsh`
- Use `--user` flag if running as non-root:
  ```bash
  docker run -it --rm \
    --user $(id -u):$(id -g) \
    -v ~/.sbsh:/home/user/.sbsh \
    docker.io/eminwux/sbsh:v0.6.0-linux-amd64 \
    sbsh
  ```

### Terminals Not Persisting

**Problem**: Terminals disappear after container restart

**Solutions**:

- Verify volume mount is correct: `docker inspect <container> | grep Mounts`
- Check that `~/.sbsh` directory is mounted as a volume
- Ensure volume is not using `--tmpfs` or ephemeral storage
- Use named volumes or bind mounts, not temporary filesystems

### Socket Connection Errors

**Problem**: Cannot connect to terminal sockets

**Solutions**:

- Verify sockets are accessible: `ls -la ~/.sbsh/run/terminals/*/socket`
- Check socket file permissions
- Ensure volume is shared between containers if using multi-container setup
- Verify network mode allows Unix socket access

### Profile Not Found

**Problem**: Profile not available in container

**Solutions**:

- Mount profile file: `-v ~/.sbsh/profiles.yaml:/root/.sbsh/profiles.yaml`
- Copy profile during image build (if using as base image)
- Verify profile path in container: `/root/.sbsh/profiles.yaml`
- Use `sb get profiles` to list available profiles

### Architecture Mismatch

**Problem**: Image won't run on host architecture

**Solutions**:

- Pull the correct architecture image:
  - AMD64: `docker.io/eminwux/sbsh:v0.6.0-linux-amd64`
  - ARM64: `docker.io/eminwux/sbsh:v0.6.0-linux-arm64`
- Use `docker manifest inspect` to check available architectures
- Verify host architecture: `uname -m`

### Container Exits Immediately

**Problem**: Container starts and immediately exits

**Solutions**:

- Use `-it` flags for interactive mode
- Check logs: `docker logs <container>`
- Verify command syntax
- Ensure CMD is appropriate for your use case

## Best Practices

1. **Use Named Volumes**: For production, use named volumes instead of bind mounts for better portability
2. **Version Pinning**: Always pin to specific version tags (e.g., `v0.6.0-linux-amd64`) instead of `latest`
3. **Profile Management**: Store profiles in version control and mount them as volumes
4. **Resource Limits**: Set appropriate CPU and memory limits for containers
5. **Security**: Avoid using `--privileged` unless absolutely necessary
6. **Backup**: Regularly backup `~/.sbsh` volumes for important terminal sessions
7. **Multi-Container**: Use Docker Compose for complex setups with multiple services

## See Also

- [Profiles Documentation](./profiles.md) - Learn how to create and customize profiles
- [CI/CD Integration](./cicd.md) - Using sbsh in CI/CD pipelines
