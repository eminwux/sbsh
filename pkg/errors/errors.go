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

// Package errors exposes the curated set of error sentinels that external
// sbsh library consumers are expected to branch on with errors.Is.
//
// Each name in this package is a direct alias for an internal sentinel in
// internal/errdefs: identity is preserved, so
//
//	errors.Is(err, sbsherrors.ErrTerminalNotFoundByID)
//
// matches errors produced anywhere in the sbsh codebase without requiring
// consumers to import internal/ packages (which Go's internal-visibility
// rules forbid anyway).
//
// Stability: pre-v1. The set of re-exports in this package may grow
// between minor releases as additional branching points are identified,
// but existing names will not change meaning until v1.
package errors

import "github.com/eminwux/sbsh/internal/errdefs"

// Terminal and client lookup sentinels. Returned by pkg/discovery and the
// discovery paths underneath pkg/spawn / pkg/rpcclient when the caller
// asked for a specific terminal or client that does not exist.
var (
	// ErrTerminalNotFoundByID is returned when a terminal is looked up by
	// ID and no terminal with that ID exists under the discovery run path.
	ErrTerminalNotFoundByID = errdefs.ErrTerminalNotFoundByID

	// ErrTerminalNotFoundByName is returned when a terminal is looked up
	// by name and no terminal with that name exists under the discovery
	// run path.
	ErrTerminalNotFoundByName = errdefs.ErrTerminalNotFoundByName

	// ErrTerminalNotFound is the generic "terminal not found" sentinel
	// returned by operations that accept either an ID or a name and
	// could not resolve either.
	ErrTerminalNotFound = errdefs.ErrTerminalNotFound

	// ErrClientNotFound is returned when a client lookup does not match
	// any live client under the discovery run path.
	ErrClientNotFound = errdefs.ErrClientNotFound
)

// Lifecycle sentinels. Returned from stop / wait operations on a terminal
// or client.
var (
	// ErrStopTerminal is returned when a stop request fails for a reason
	// other than timeout (e.g. the underlying signal could not be
	// delivered). Callers that want to distinguish "graceful stop did not
	// complete in time" from "stop failed outright" should branch on
	// ErrStopTimeout first, then fall back to ErrStopTerminal.
	ErrStopTerminal = errdefs.ErrStopTerminal

	// ErrStopTimeout is returned when a terminal did not exit within the
	// configured stop timeout. This is distinct from ErrStopTerminal
	// because callers may want to escalate (e.g. issue a forced stop)
	// only on timeout, not on hard failure.
	ErrStopTimeout = errdefs.ErrStopTimeout

	// ErrWaitOnReady is returned when waiting for a terminal or client
	// to become Ready fails (context cancelled, child exited before
	// ready, etc). Intended for library callers that want to fail fast
	// on readiness-related spawn failures rather than inspect the full
	// state machine.
	ErrWaitOnReady = errdefs.ErrWaitOnReady
)

// Streaming sentinels. Returned from subscribe-style RPCs when the
// consumer cannot keep up with the producer.
var (
	// ErrSubscriberLagged is returned to a subscriber that fell behind
	// the producer's ring buffer and was disconnected. Callers typically
	// reconnect and resume from the latest offset.
	ErrSubscriberLagged = errdefs.ErrSubscriberLagged
)

// Configuration sentinels. Returned when a required configuration value
// is missing from the caller's spec / environment.
var (
	// ErrRunPathRequired is returned when an operation needs the sbsh
	// run path (socket/state directory) and neither the spec, the
	// environment, nor the default resolution could produce one.
	// External library callers must provide StateRoot explicitly; they
	// should treat this as a programming error.
	ErrRunPathRequired = errdefs.ErrRunPathRequired
)
