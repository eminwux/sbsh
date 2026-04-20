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

// Package pidutil exposes a stable per-process identifier used to detect PID
// reuse: after a process exits, the kernel may recycle its PID for an
// unrelated process, so callers that persist PIDs and later signal them must
// reconfirm identity before delivering the signal.
//
// On Linux the identifier is the kernel-reported start time
// (/proc/<pid>/stat field 22 — clock ticks since boot); on platforms without
// an equivalent zero-cost source the identifier is 0 ("unknown") and
// identity checks degrade to "trust the PID". Stored alongside the PID in
// metadata, the identifier lets callers reject stale PIDs before signalling.
package pidutil

// StartTime returns an opaque identifier for the running instance with the
// given PID. Two calls for the same live process return the same value;
// calls separated by a process exit (and PID recycle) return different
// values. A return of (0, nil) means the platform does not expose a usable
// per-process identifier and callers should skip the identity check.
func StartTime(pid int) (uint64, error) {
	if pid <= 0 {
		return 0, nil
	}
	return readStartTime(pid)
}

// Match reports whether the process currently holding pid is the same one
// that produced expected. An expected value of 0 means "no token recorded"
// (e.g. metadata written before this check existed, or a platform that does
// not support it) and Match returns true so callers fall back to the raw
// PID. Errors from the platform reader (most notably ESRCH/"no such file")
// are surfaced as (false, err) so callers can distinguish "process gone"
// from "unable to determine".
func Match(pid int, expected uint64) (bool, error) {
	if expected == 0 {
		return true, nil
	}
	got, err := StartTime(pid)
	if err != nil {
		return false, err
	}
	if got == 0 {
		// Platform doesn't expose the token but the metadata recorded one;
		// can't verify, so refuse to confirm a match.
		return false, nil
	}
	return got == expected, nil
}
