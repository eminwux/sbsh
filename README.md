# 🚂 sbsh - tty supervisor

sbsh is a terminal supervisor that manages persistent sessions.
Each session is an independent, long-lived environment that continues running even when no supervisor is attached. Supervisors can attach to existing sessions, create new ones, observe activity, or send input and commands through an API.

sbsh gives terminals persistence, structure, and visibility without changing how programs run inside them. It separates the session (the living environment) from the supervisor (the controller), allowing both humans and programs to interact with terminals in a durable and programmable way.

## Features

- Persistent terminal sessions that survive disconnects and supervisor restarts
- Profiles that define how sessions are launched, including commands, environment variables, and policies. See [examples/profiles/local.yaml](examples/profiles/local.yaml).
- Session discovery and reattachment to resume work without losing state
- Multiple supervisors or API clients can connect to the same session concurrently
- Structured logs and metadata for every session, making state and history easy to inspect
- Clean separation between the session (the environment) and the supervisor (the controller)
- Programmable API that lets external programs create, control, or analyze sessions for automation and integration

## Install

```bash
# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.1.4/sbsh-linux-amd64 && \
chmod +x sbsh && \
sudo mv sbsh /usr/local/bin/ && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

## Quick Start

```bash
# Start a supervisor with the default configuration
sbsh

# Detach the session
sb detach
or
sb d

# List active sessions
sb sessions list
or
sb s l

# List all sessions
sb sessions list -a
or
sb s l -a

# Attach to a running session
sb attach --id <session-id>
or
sb a --id <session-id>

# List available profiles
sb profiles list
or
sb s l

# Start a supervisor with a given profile
sbsh --profile prod
or
sbsh -p prod
```

# Why sbsh exists

Terminals are still treated as ephemeral. Once a shell closes or a connection drops, the environment dies with it.

sbsh changes that by giving terminals persistence and structure.

Each sbsh session is an independent environment that continues running even if the supervisor exits or restarts. Sessions can be discovered later, reattached, observed, or controlled by new supervisors or API clients. They expose structured logs and metadata for inspection, treating the terminal as an addressable system object rather than a temporary process.

sbsh is built on the idea that terminals deserve supervision.

Where operating systems supervise daemons and containers, sbsh supervises the terminal itself, the space where people and programs interact. Its goal is to make terminal sessions durable, observable, and programmable, bridging the gap between interactive and automated work.

# Philosophy

sbsh follows the traditional Unix philosophy: build simple tools that do one thing well and compose naturally with others. It treats supervision not as orchestration or complexity, but as clarity — giving interactive work the same discipline that background services have enjoyed for decades.

As Ken Thompson once said, “One of my most productive days was throwing away 1,000 lines of code.”
sbsh embraces that mindset by keeping its design small, transparent, and essential: a single program that brings persistence and structure to the terminal.

# Status and Roadmap

sbsh is under active development, with a focus on correctness, portability, and clear abstractions before adding integrations.

Work in progress, planned features, and the project roadmap can be found in the [ROADMAP.md](./ROADMAP.md) file. This file is regularly updated with bugs, priorities, and upcoming improvements.

# Contribute

sbsh is an open project that welcomes thoughtful contributions. The goal is to build a simple, reliable foundation for persistent terminal sessions, not a large framework. Discussions, code reviews, and design proposals are encouraged, especially around clarity, portability, and correctness. If you want to contribute, open an issue or pull request with a clear explanation of the problem or improvement, and keep the focus on making the supervisor and session model more robust and maintainable.

# License

Apache License 2.0

© 2025 Emiliano Spinella (eminwux)
