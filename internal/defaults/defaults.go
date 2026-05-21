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

const (
	TerminalsRunPath = "terminals"
	ClientsRunPath   = "clients"
)

// Capture rotation + retention knobs. Hard-coded for now, configurable later
// (same "hard-code now, configurable later" precedent #71 set for the read
// path). Rotation bounds per-segment size; retention bounds total on-disk use,
// since rotation alone only caps each file. See internal/capture and the
// rotating writer in internal/terminal/terminalrunner.
const (
	// CaptureSegmentMaxBytes is the per-segment rotation threshold: once the
	// live segment reaches this size it is closed, renamed to a deterministic
	// sibling, and a fresh live segment opens at the canonical path.
	CaptureSegmentMaxBytes int64 = 8 << 20 // 8 MiB

	// CaptureRetentionMaxSegments bounds how many closed segments are kept;
	// the oldest are pruned first. Zero disables count-based pruning.
	CaptureRetentionMaxSegments = 8

	// CaptureRetentionMaxBytes bounds the total size of closed segments;
	// the oldest are pruned until the total fits. Zero disables byte-based
	// pruning. The live segment is never pruned.
	CaptureRetentionMaxBytes int64 = 64 << 20 // 64 MiB
)
