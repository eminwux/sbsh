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

// Package capture reads the capture files an sbsh terminal writes: the
// transcript of everything its PTY printed. It lists a capture's segments,
// opens them (as stored, or decompressed), sniffs the capture format, and
// decodes asciicast captures. It is the reader `sb read` and attach
// --full-capture use, and it works on a capture directory with no sbsh
// process running.
//
// # Segment naming (contract)
//
// A capture is identified by its canonical path (TerminalSpec.CaptureFile).
// The writer rotates it into segments to bound per-file size and prunes the
// oldest closed segments to bound disk use:
//
//   - The canonical path is the live segment: append-only, never compressed.
//     A raw `cat` of it shows only the current tail.
//   - A closed segment is a sibling named "<canonical>.NNNNNN", where NNNNNN is
//     a decimal sequence number, zero-padded to six digits (wider once it
//     outgrows them). Lower numbers are older. On rotation the live segment is
//     renamed to the next sequence number (NextSeq), then compressed to
//     "<canonical>.NNNNNN.gz".
//   - During compression the raw and ".gz" forms of one segment can coexist
//     briefly. They are the same segment; the raw form wins.
//   - Any other sibling — a non-digit suffix, an in-flight ".gz.tmp" — is not
//     part of the capture.
//
// The full capture is the closed segments in ascending sequence order, then
// the live segment, each decompressed and concatenated (Segments, Open,
// ReadAll, Dump).
//
// # Formats
//
// A raw capture is the PTY bytes, unchanged. An asciicast capture is an
// asciicast v2 stream (https://docs.asciinema.org/manual/asciicast/v2/): JSON
// header lines and `[time, code, data]` event lines, one per line. Sniff and
// IsAsciicast tell them apart from the first line of the stream. In an
// asciicast capture:
//
//   - Every segment begins with a header, so the capture stays decodable after
//     the oldest segments are pruned. The header's width and height are the
//     terminal size in effect when it was written.
//   - Each terminal run on a reused capture path begins with a header whose
//     timestamp is the run's start time; within a run, event times never
//     decrease. Segment headers repeat their run's timestamp.
//   - EventOutput events carry output text; EventResize events carry the new
//     size as "<cols>x<rows>".
//   - No line is ever split across segments.
//
// # Timing rule (contract)
//
// An event's wall-clock time is the Timestamp of the nearest preceding header
// plus the event's Time.
package capture
