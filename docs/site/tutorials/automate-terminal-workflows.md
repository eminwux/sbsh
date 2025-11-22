# Automate Terminal Workflows

This tutorial shows you how to automate terminal workflows using sbsh profiles and scripts.

## Prerequisites

- sbsh installed
- Basic shell scripting knowledge

## Automation Patterns

### Pattern 1: Automated Setup Script

Create a script that sets up a terminal environment:

```bash
#!/bin/bash
# setup-dev-env.sh

# Start terminal with profile
sbsh terminal --name dev-setup -p dev-profile

# Wait for terminal to be ready
sleep 2

# Attach and run setup commands
sb attach dev-setup <<EOF
npm install
npm run build
EOF
```

### Pattern 2: CI/CD Integration

Use sbsh in CI/CD pipelines for reproducible environments:

```yaml
# .github/workflows/test.yml
name: Tests
on: [push]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup sbsh
        run: |
          wget -O sbsh https://github.com/eminwux/sbsh/releases/download/v0.6.0/sbsh-linux-amd64
          chmod +x sbsh && sudo install -m 0755 sbsh /usr/local/bin/sbsh
          sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb

      - name: Create test profile
        run: |
          mkdir -p ~/.sbsh
          cat > ~/.sbsh/profiles.yaml <<PROFILE
          apiVersion: sbsh/v1beta1
          kind: TerminalProfile
          metadata:
            name: ci-test
          spec:
            shell:
              cwd: "${{ github.workspace }}"
              env:
                CI: "true"
          PROFILE

      - name: Run tests
        run: sbsh -p ci-test --name "ci-${{ github.run_id }}"
```

See the [CI/CD Guide](../guides/cicd.md) for complete examples.

### Pattern 3: Scheduled Tasks

Use systemd timers or cron with sbsh:

```bash
# /etc/cron.daily/backup
#!/bin/bash
sbsh terminal --name daily-backup -p backup-profile
```

### Pattern 4: Profile-Based Automation

Create profiles that automate common tasks:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: auto-deploy
spec:
  shell:
    cwd: "$HOME/projects/myapp"
    env:
      DEPLOY_ENV: "production"
  stages:
    onInit:
      - script: git pull
      - script: npm install
      - script: npm run build
      - script: ./deploy.sh
```

## Scripting Examples

### Wait for Terminal Ready

```bash
#!/bin/bash
TERMINAL_NAME="my-terminal"

# Start terminal
sbsh terminal --name "$TERMINAL_NAME" -p my-profile

# Wait for terminal to be ready
while ! sb get terminal "$TERMINAL_NAME" > /dev/null 2>&1; do
  sleep 0.5
done

echo "Terminal $TERMINAL_NAME is ready"
```

### Batch Terminal Operations

```bash
#!/bin/bash
# Start multiple terminals
for env in dev staging prod; do
  sbsh terminal --name "terraform-$env" -p "terraform-$env"
done

# List all started terminals
sb get terminals
```

### Terminal Health Check

```bash
#!/bin/bash
# Check terminal status
TERMINAL_NAME="my-terminal"

STATUS=$(sb get terminal "$TERMINAL_NAME" -o json | jq -r '.status')

if [ "$STATUS" = "Ready" ] || [ "$STATUS" = "Attached" ]; then
  echo "Terminal is healthy"
  exit 0
else
  echo "Terminal is not healthy: $STATUS"
  exit 1
fi
```

## Integration with Other Tools

### Docker Compose

```yaml
# docker-compose.yml
version: "3.8"
services:
  sbsh:
    image: docker.io/eminwux/sbsh:v0.6.0-linux-amd64
    volumes:
      - ~/.sbsh:/root/.sbsh
    command: sbsh terminal --name automated-task -p my-profile
```

### Kubernetes Jobs

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: sbsh-automated-task
spec:
  template:
    spec:
      containers:
        - name: sbsh
          image: docker.io/eminwux/sbsh:v0.6.0-linux-amd64
          command:
            ["/bin/sbsh", "terminal", "--name", "k8s-task", "-p", "my-profile"]
```

## Best Practices

1. **Use named terminals**: Always use `--name` for automation
2. **Check terminal status**: Verify terminals are ready before use
3. **Handle errors**: Check exit codes and terminal states
4. **Clean up**: Prune old automated terminals
5. **Log everything**: Use terminal logs for debugging

## Related Documentation

- [CI/CD Guide](../guides/cicd.md) - Complete CI/CD integration
- [Container Usage](../guides/container.md) - Docker and Kubernetes
- [Profiles Guide](../guides/profiles.md) - Profile configuration
