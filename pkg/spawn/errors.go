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

package spawn

import "errors"

var (
	// ErrBinaryPathRequired is returned when a spawn call is issued without
	// a non-empty binary path in its Options. Library callers must point
	// spawn at a specific sbsh/sb executable — there is no PATH lookup
	// fallback, so that state-root-style isolation is not silently broken
	// by whatever happens to be first on $PATH.
	ErrBinaryPathRequired = errors.New("binary path is required")

	// ErrSpecRequired is returned when NewTerminal is called with a nil
	// TerminalSpec.
	ErrSpecRequired = errors.New("terminal spec is required")

	// ErrDocRequired is returned when NewClient is called with a nil
	// ClientDoc.
	ErrDocRequired = errors.New("client doc is required")

	// ErrSocketPathRequired is returned when the provided spec/doc does
	// not carry a concrete control-socket path. spawn does not invent one
	// — callers are expected to fill it in (usually via pkg/builder in
	// PR-D, or profile.BuildTerminalSpec today).
	ErrSocketPathRequired = errors.New("socket path is required")

	// ErrAttachTargetRequired is returned when NewClient is called
	// without a terminal ID or name to attach to.
	ErrAttachTargetRequired = errors.New("attach target (terminal id or name) is required")

	// ErrUnsupportedClientMode is returned when NewClient is called with
	// a ClientMode other than AttachToTerminal. RunNewTerminal mode is
	// expected to be handled by composing NewTerminal + NewClient; it is
	// not a spawn-level primitive in this release.
	ErrUnsupportedClientMode = errors.New("unsupported client mode")

	// ErrProcessExited is returned from WaitReady when the spawned child
	// exits before it ever reports Ready. The handle's WaitClose will
	// then return the underlying exit error.
	ErrProcessExited = errors.New("spawned process exited before ready")

	// ErrReadyTimeout is returned from WaitReady when the readiness
	// deadline expires with the child still running but not yet Ready.
	ErrReadyTimeout = errors.New("timed out waiting for readiness")
)
