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

// Package capture defines the on-disk layout of a terminal's PTY transcript
// and the helpers that reassemble it for readers.
//
// A capture is rotated into segments to bound per-file size, with retention
// pruning the oldest closed segments to bound total disk use (see the rotating
// writer in internal/terminal/terminalrunner). The layout is:
//
//   - The canonical path (the operator-facing capture file) is always the
//     live, append-only, raw segment — the current tail of the transcript.
//   - Closed segments are deterministic siblings named "<canonical>.NNNNNN",
//     where NNNNNN is a zero-padded, monotonically increasing sequence number.
//     Lower sequence numbers are older.
//
// Contract: a raw `cat` of the canonical path shows the live tail only.
// Reassembling the full history is the job of the readers here (used by
// `sb read`, attach `--full-capture`, and any other consumer of full
// transcript): ordered closed segments oldest-first, then the live segment.
//
// The segment matcher requires an all-digit suffix, so unrelated siblings
// (e.g. a future compressed "<canonical>.NNNNNN.gz") are ignored rather than
// spliced into the stream.
package capture

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// seqWidth is the zero-padding width for closed-segment sequence numbers. It
// is cosmetic — readers parse the full numeric suffix and order numerically —
// so a sequence that outgrows the width still sorts correctly.
const seqWidth = 6

// SegmentPath returns the deterministic closed-segment path for seq, a sibling
// of canonical: "<canonical>.NNNNNN".
func SegmentPath(canonical string, seq uint64) string {
	return fmt.Sprintf("%s.%0*d", canonical, seqWidth, seq)
}

// Segment is a closed capture segment on disk.
type Segment struct {
	Path string
	Seq  uint64
	Size int64
}

// ClosedSegments returns the closed segments siblings of canonical, ordered
// oldest-first (ascending sequence). It does not include the canonical (live)
// path. A missing directory yields an empty slice and no error.
func ClosedSegments(canonical string) ([]Segment, error) {
	dir := filepath.Dir(canonical)
	base := filepath.Base(canonical)
	prefix := base + "."

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read capture dir %q: %w", dir, err)
	}

	var segs []Segment
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := name[len(prefix):]
		// Require an all-digit suffix so unrelated siblings (e.g. a
		// compressed ".NNNNNN.gz") are not spliced into the stream.
		seq, perr := strconv.ParseUint(suffix, 10, 64)
		if perr != nil {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			// Raced away between ReadDir and Info; skip it.
			continue
		}
		segs = append(segs, Segment{
			Path: filepath.Join(dir, name),
			Seq:  seq,
			Size: info.Size(),
		})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].Seq < segs[j].Seq })
	return segs, nil
}

// NextSeq returns the sequence number a fresh rotation should assign: one past
// the highest existing closed segment, or 0 when none exist. Lets a writer
// resume a deterministic, gap-free ordering across process restarts.
func NextSeq(canonical string) (uint64, error) {
	segs, err := ClosedSegments(canonical)
	if err != nil {
		return 0, err
	}
	if len(segs) == 0 {
		return 0, nil
	}
	return segs[len(segs)-1].Seq + 1, nil
}

// OrderedPaths returns the full reassembly order: closed segments oldest-first,
// then the canonical (live) path if it exists on disk.
func OrderedPaths(canonical string) ([]string, error) {
	segs, err := ClosedSegments(canonical)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(segs)+1)
	for _, s := range segs {
		paths = append(paths, s.Path)
	}
	if _, serr := os.Stat(canonical); serr == nil {
		paths = append(paths, canonical)
	} else if !os.IsNotExist(serr) {
		return nil, fmt.Errorf("stat capture file %q: %w", canonical, serr)
	}
	return paths, nil
}

// ReadAll reassembles the entire transcript — closed segments oldest-first,
// then the live segment — into one byte slice. A capture that was never
// written (no segments, no live file) yields a nil slice and no error.
func ReadAll(canonical string) ([]byte, error) {
	paths, err := OrderedPaths(canonical)
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, p := range paths {
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				// Pruned or rotated away between listing and read; skip.
				continue
			}
			return nil, fmt.Errorf("read capture segment %q: %w", p, rerr)
		}
		out = append(out, data...)
	}
	return out, nil
}

// Dump streams the reassembled transcript to w in order: closed segments
// oldest-first, then the live segment. A capture that was never written
// produces no output and no error (an absent file is indistinguishable from a
// never-written one).
func Dump(canonical string, w io.Writer) error {
	paths, err := OrderedPaths(canonical)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if derr := dumpOne(p, w); derr != nil {
			return derr
		}
	}
	return nil
}

func dumpOne(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Pruned or rotated away between listing and open; skip.
			return nil
		}
		return fmt.Errorf("open capture segment %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, cerr := io.Copy(w, f); cerr != nil {
		return fmt.Errorf("read capture segment %q: %w", path, cerr)
	}
	return nil
}
