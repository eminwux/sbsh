# sbsh: Terminal-as-Code

![status: active](https://img.shields.io/badge/status-active-blue)
![state: beta](https://img.shields.io/badge/state-beta-orange)
![license: apache2](https://img.shields.io/badge/license-Apache%202.0-green)

sbsh brings **Terminal-as-Code** to your workflow: define terminal environments declaratively with YAML manifests that can be version-controlled, shared, and reused across your team. Each profile specifies environment variables, lifecycle hooks, startup commands, and visual prompts, ensuring consistent setups across local machines, jump hosts, and CI/CD pipelines.

**Demo - Launch a Terraform production workspace: profile automatically configures environment, workspace, and shows red warning prompt**

![Demo - Launch a Terraform production workspace](assets/demo-terraform.gif)

## Why sbsh

_Terminal-as-Code_ brings the discipline of Infrastructure-as-Code to interactive environments:

- **Reproducible setups**: Profiles ensure identical environments across team members and CI/CD
- **Durable terminals**: Survive crashes and network interruptions
- **Team collaboration**: Share terminals with teammates for pair programming and collaborative debugging
- **Environment safety**: Color-coded prompts and standardized configurations prevent mistakes
- **Automation ready**: Programmatic API and structured logs enable integration with existing tooling

## Key Features

### Declarative Profiles

Define terminal environments as YAML manifests that can be version-controlled and shared:

```yaml
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: terraform-prd
spec:
  shell:
    env:
      TF_VAR_environment: "prd"
    prompt: '"\[\e[1;31m\]sbsh(terraform-prd) \[\e[1;32m\]\u@\h\[\e[0m\]:\w\$ "'
  stages:
    onInit:
      - script: terraform workspace use prd
      - script: terraform init
```

### Persistent Terminals

Terminals survive network drops, supervisor restarts, and accidental disconnects. Reattach anytime with `sb attach <name>`.

### Multi-Attach Support

Multiple people can attach to the same terminal for pair programming and collaborative debugging. Built-in discovery makes terminals shareable across your team.

### Clean Architecture

sbsh separates the terminal (your work environment) from the supervisor (the controller), making terminals as durable and manageable as background services. Terminals run independently and survive supervisor crashes.

## Quick Start

Get sbsh up and running in minutes:

```bash
# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.5.0/sbsh-linux-amd64 && \
chmod +x sbsh && \
sudo install -m 0755 sbsh /usr/local/bin/sbsh && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb

# Start your first terminal
sbsh
```

See the [Getting Started](getting-started.md) guide for detailed installation and setup instructions.

## Philosophy

sbsh follows the traditional Unix philosophy: build simple tools that do one thing well and compose naturally with others. It treats supervision not as orchestration or complexity, but as clarity, giving interactive work the same discipline that background services have enjoyed for decades.

sbsh applies the same principles of Infrastructure-as-Code to interactive environments, turning terminals into reproducible, declarative, and shareable units of work.

## Documentation

- **[Getting Started](getting-started.md)** - Install and configure sbsh
- **[Concepts](concepts/terminals.md)** - Understand terminals, profiles, and supervisors
- **[Guides](guides/profiles.md)** - Comprehensive guides for profiles, CI/CD, and containers
- **[CLI Reference](cli/commands.md)** - Complete command documentation
- **[Tutorials](tutorials/create-your-first-profile.md)** - Step-by-step tutorials

## Status

sbsh is under active development, with a focus on correctness, portability, and clear abstractions before adding integrations.

See the [Roadmap](roadmap.md) for work in progress, planned features, and the project roadmap.

## License

Apache License 2.0

© 2025 Emiliano Spinella (eminwux)
