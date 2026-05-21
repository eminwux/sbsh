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

package pidutil

import "testing"

// TestMatch_NonPositivePidWithToken covers the got==0 branch: StartTime
// returns (0, nil) for a non-positive PID, so Match cannot confirm a recorded
// token and returns false.
func TestMatch_NonPositivePidWithToken(t *testing.T) {
	ok, err := Match(0, 12345)
	if err != nil {
		t.Fatalf("Match(0, token) error: %v", err)
	}
	if ok {
		t.Fatal("Match(0, token) = true; want false (cannot verify)")
	}
}

// TestMatch_DeadPidSurfacesError covers Match's error-propagation branch: a PID
// with no live process makes the platform reader fail, and Match surfaces
// (false, err) so callers distinguish "gone" from "matched".
func TestMatch_DeadPidSurfacesError(t *testing.T) {
	// A very high PID is almost certainly not live; StartTime fails to read
	// its per-process token.
	const deadPid = 0x7ffffffe
	tok, err := StartTime(deadPid)
	if err == nil {
		// On a platform without a per-process token, StartTime returns
		// (0, nil) and Match degrades gracefully — nothing to assert.
		t.Skipf("platform returned token %d with no error for dead pid; skipping", tok)
	}
	ok, err := Match(deadPid, 12345)
	if err == nil {
		t.Fatal("Match(deadPid, token) err = nil; want a reader error")
	}
	if ok {
		t.Fatal("Match(deadPid, token) = true; want false")
	}
}
