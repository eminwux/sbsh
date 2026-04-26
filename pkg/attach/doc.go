// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// Package attach exposes an in-process façade for sbsh's interactive
// "sb attach" loop, so external Go programs can drive an attach session
// against a running terminal control socket without spawning the sb
// binary as a subprocess.
//
// The single public entrypoint is Run(ctx, Options). It connects to the
// terminal control socket given by Options.SocketPath, runs the
// bidirectional PTY copy loop (caller-supplied stdin/stdout/stderr),
// installs the SIGWINCH-driven resize forwarder, honours the detach
// keystroke (^] twice, opt-out via DisableDetachKeystroke), and returns
// when the session ends — either because the remote side closed, the
// user detached, or the caller cancelled ctx.
//
// Options.Stdin must be a TTY-backed *os.File (the loop puts it in raw
// mode and reads window size from it via TIOCGWINSZ). Stdout/Stderr can
// be any *os.File; embedders typically pass os.Stdout / os.Stderr.
//
// The package allocates a private Unix-domain control socket for the
// embedded client under os.TempDir() for the lifetime of the call and
// removes it on return. SIGINT/SIGTERM are intentionally NOT trapped
// here — the caller is in charge of cancelling ctx in response to
// whichever signals make sense for their program. cmd/sb/attach wires
// signal.NotifyContext on top of pkg/attach so the CLI keeps its
// existing Ctrl-C / SIGTERM behaviour.
//
// Stability: pre-v1. Field names and defaults may change between minor
// releases until the umbrella issue (#118) closes.
package attach
