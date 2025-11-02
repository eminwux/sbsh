# 🚂 sbsh - tty supervisor

sbsh is a terminal supervisor that manages persistent terminal sessions.
Each terminal is an independent, long-lived environment that continues running even when no supervisor is attached. Supervisors can attach to existing terminals, create new ones, observe activity, or send input and commands through an API.

sbsh gives terminals persistence, structure, and visibility without changing how programs run inside them. It separates the session (the living environment) from the supervisor (the controller), allowing both humans and programs to interact with terminals in a durable and programmable way.

## Features

- Persistent terminal sessions that survive disconnects and supervisor restarts
- Profiles that define how terminals are launched, including commands, environment variables, and policies. See [examples/profiles/local.yaml](examples/profiles/local.yaml).
- Session discovery and reattachment to resume work without losing state
- Multiple supervisors or API clients can connect to the same session concurrently
- Structured logs and metadata for every session, making state and history easy to inspect
- Clean separation between the session (the environment) and the supervisor (the controller)
- Programmable API that lets external programs create, control, or analyze sessions for automation and integration

# Quick Start

## Install

```bash
# Install sbsh
curl -L -o sbsh https://github.com/eminwux/sbsh/releases/download/v0.2.0/sbsh-linux-amd64 && \
chmod +x sbsh && \
sudo mv sbsh /usr/local/bin/ && \
sudo ln -f /usr/local/bin/sbsh /usr/local/bin/sb
```

## Autocomplete

```bash
cat >> ~/.bashrc <<EOF
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
EOF
```

## Usage

### Launch sbsh

Launch a supervisor with the default configuration

```
eminwux@capgood:~$ sbsh
To detach, press ^] twice
bash-5.2$ export PS1="[sbsh-$SBSH_SES_ID] \u@\h:\w$ "
[sbsh-35af93de] inwx@capgood:~/projects/sbsh$ cd ~
[sbsh-35af93de] inwx@capgood:~$
```

### Detach the terminal

Once inside the terminal, press Ctrl-] twice (shown as ^]) to detach the supervisor and return to your shell. The session will continue running and can be reattached later.

```
[sbsh-ee4196b6] eminwux@capgood:~$ sb d
To detach, press ^] twice
bash-5.2$ export PS1="[sbsh-$SBSH_SES_ID] \u@\h:\w$ "
[sbsh-35af93de] inwx@capgood:~/projects/sbsh$ cd ~
[sbsh-35af93de] inwx@capgood:~$
Detached
eminwux@capgood:~$
```

### List active terminals

List active terminals

```
eminwux@capgood:~$ sb get terminals
ID        NAME           PROFILE  CMD                           TTY          STATUS  ATTACHERS  LABELS
bbb4b457  quick_samwise  default  /bin/bash --norc --noprofile  /dev/pts/12  Ready   None       none
eminwux@capgood:~$
```

### Attach to a running terminal

```
eminwux@capgood:~$ sb attach quick_samwise
To detach, press ^] twice
bash-5.2$ export PS1="[sbsh-$SBSH_SES_ID] \u@\h:\w$ "
[sbsh-bbb4b457] inwx@capgood:~/projects/sbsh$ cd ~
[sbsh-bbb4b457] inwx@capgood:~$
```

### List available profiles

See [examples/profiles/local.yaml](examples/profiles/local.yaml) to define your own profiles.

```
eminwux@capgood:~$ sb get profiles
NAME              TARGET  ENVVARS  CMD
default           local   3 vars   /bin/bash --norc --noprofile
k8s-default       local   4 vars   /bin/bash
terraform-prd     local   2 vars   /bin/bash
k8s-pod           local   1 vars   /usr/local/bin/kubectl run -ti --rm --image debian:stable-slim ephemeral
docker-container  local   0 vars   /usr/bin/docker run --rm -ti debian:stable-slim /bin/bash
ssh-pk            local   0 vars   /usr/bin/ssh -t pk
eminwux@capgood:~$
```

### Start a supervisor with a given profile

```
eminwux@capgood:~$ sbsh -p docker-container
sbsh root@a5ad0ede7212:~$
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
