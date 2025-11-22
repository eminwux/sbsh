# Frequently Asked Questions

Common questions about sbsh and how to use it.

## General

### What is sbsh?

sbsh is a Terminal-as-Code system that provides persistent, replayable, and shareable terminal sessions. It allows you to define terminal environments declaratively using YAML profiles, ensuring consistent setups across team members and CI/CD pipelines.

### How is sbsh different from screen or tmux?

sbsh treats terminals as independent, managed services with declarative configuration, built-in discovery, and programmatic control. Unlike screen and tmux, sbsh has no central server process—each terminal runs autonomously. See the [Comparison Guide](guides/comparison.md) for detailed differences.

### Is sbsh production-ready?

sbsh is currently in beta. It's under active development with a focus on correctness, portability, and clear abstractions. See the [Roadmap](roadmap.md) for current status and planned features.

## Installation

### What platforms does sbsh support?

sbsh supports Linux, macOS, and FreeBSD. Both amd64 and arm64 architectures are supported. See the [Installation](install/prerequisites.md) section for platform-specific instructions.

### How do I install sbsh on Windows?

Windows support is planned but not yet available. You can use sbsh in WSL (Windows Subsystem for Linux) or Docker containers. See the [Container Usage Guide](guides/container.md) for details.

## Usage

### How do I detach from a terminal?

Press `Ctrl-]` twice, or run `sb detach` from within the terminal. The terminal will continue running in the background.

### Can multiple people attach to the same terminal?

Yes! sbsh supports multi-attach by default. Multiple supervisors can connect to the same terminal concurrently. See [Multi-Attach](concepts/multi-attach.md) for details.

### How do I find terminals after disconnecting?

Use `sb get terminals` to list all active terminals. You can attach by name or ID: `sb attach <name>` or `sb attach <id>`.

### What happens if the supervisor crashes?

Terminals are independent processes and continue running even if the supervisor exits. You can reattach with `sb attach <name>`.

## Profiles

### Where are profiles stored?

Profiles are stored in `~/.sbsh/profiles.yaml` by default. You can specify a different location using the `SBSH_PROFILES_FILE` environment variable or `--profiles-file` flag.

### Can I use multiple profile files?

Currently, sbsh supports a single `profiles.yaml` file. Multiple profiles are defined in the same file, separated by `---` (YAML document separator). See the [Profiles Guide](guides/profiles.md) for details.

### How do I create a profile?

See the [Create Your First Profile](tutorials/create-your-first-profile.md) tutorial for step-by-step instructions.

## Troubleshooting

### Profile not found

- Check the profile name matches exactly (case-sensitive)
- Verify `~/.sbsh/profiles.yaml` exists
- Run `sb get profiles` to see available profiles
- Check if you're using a custom profiles file path

### Terminal not starting

- Check working directory exists: `cwd` must be valid
- Verify commands in `onInit` are available in the environment
- Check file permissions and paths
- Review terminal logs in `~/.sbsh/run/terminals/<name>/`

### Environment variables not working

- If `inheritEnv: false`, make sure variables are in the `env` map
- Use `$HOME` instead of `~` in env values
- Quote variable values that contain spaces or special characters
- Numbers must be strings: `"5000"` not `5000`

## See Also

- [Getting Started](getting-started.md) - Installation and basic usage
- [Concepts](concepts/terminals.md) - Understanding sbsh architecture
- [Profiles Guide](guides/profiles.md) - Creating and managing profiles
- [CLI Reference](cli/commands.md) - Complete command documentation
