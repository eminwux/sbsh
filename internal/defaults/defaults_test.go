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

package defaults

import "testing"

// The defaults package declares only constants and has no executable
// statements, so `go test -cover` reports "no statements" for it (exempt
// from the ≥80% bar per the issue). These assertions guard the on-disk and
// rotation/retention contract that internal/capture and the terminalrunner
// depend on, so an accidental edit to the values is caught here.

func TestRunPathConstants(t *testing.T) {
	if TerminalsRunPath != "terminals" {
		t.Errorf("TerminalsRunPath = %q, want terminals", TerminalsRunPath)
	}
	if ClientsRunPath != "clients" {
		t.Errorf("ClientsRunPath = %q, want clients", ClientsRunPath)
	}
}

func TestCaptureRotationRetentionKnobs(t *testing.T) {
	if CaptureSegmentMaxBytes != 8<<20 {
		t.Errorf("CaptureSegmentMaxBytes = %d, want %d (8 MiB)", CaptureSegmentMaxBytes, 8<<20)
	}
	if CaptureRetentionMaxSegments != 8 {
		t.Errorf("CaptureRetentionMaxSegments = %d, want 8", CaptureRetentionMaxSegments)
	}
	if CaptureRetentionMaxBytes != 64<<20 {
		t.Errorf("CaptureRetentionMaxBytes = %d, want %d (64 MiB)", CaptureRetentionMaxBytes, 64<<20)
	}
	// Retention must bound at least one full segment, else rotation churns.
	if CaptureRetentionMaxBytes < CaptureSegmentMaxBytes {
		t.Errorf("retention (%d) < segment size (%d): retention cannot hold a segment",
			CaptureRetentionMaxBytes, CaptureSegmentMaxBytes)
	}
}
