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
//     Lower sequence numbers are older. On rotation a closed segment is gzip-
//     compressed to "<canonical>.NNNNNN.gz" to stretch the retention budget;
//     the live segment always stays raw and seekable.
//
// Contract: a raw `cat` of the canonical path shows the live tail only.
// Reassembling the full history is the job of the readers here (used by
// `sb read`, attach `--full-capture`, and any other consumer of full
// transcript): ordered closed segments oldest-first, then the live segment.
// Readers transparently decompress ".gz" closed segments, dispatching on file
// extension, so a capture spanning gzip-closed and raw-live segments reads
// back as the original byte stream.
//
// The segment matcher accepts both the raw "<canonical>.NNNNNN" form and the
// compressed "<canonical>.NNNNNN.gz" form (collapsing the two to one segment
// when both transiently coexist mid-rotation, preferring the raw copy).
// Other siblings (a non-digit suffix, an in-flight ".gz.tmp") are ignored
// rather than spliced into the stream.
package capture

import (
	"compress/gzip"
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

// GzSuffix is appended to a closed segment's path once it has been gzip-
// compressed: "<canonical>.NNNNNN.gz". Compression is sequential-read only
// (readers io.Copy each segment end-to-end, never seeking), so gzip's lack of
// a seek index is a non-issue and it adds no dependency beyond the stdlib.
const GzSuffix = ".gz"

// SegmentPath returns the deterministic closed-segment path for seq, a sibling
// of canonical: "<canonical>.NNNNNN".
func SegmentPath(canonical string, seq uint64) string {
	return fmt.Sprintf("%s.%0*d", canonical, seqWidth, seq)
}

// Segment is a closed capture segment on disk.
type Segment struct {
	Path string
	Seq  uint64
	// Size is the on-disk size: for a compressed segment this is the gzip
	// size, so byte-based retention bounds actual disk use.
	Size int64
	// Compressed is true when Path is a ".gz" segment that readers must
	// decompress.
	Compressed bool
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

	// Collapse same-seq raw + ".gz" siblings (transient during rotation's
	// compress step, when both copies briefly coexist) to one segment,
	// preferring the raw copy: it is the original bytes and skips a decompress.
	bySeq := make(map[uint64]Segment)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := name[len(prefix):]
		compressed := false
		if strings.HasSuffix(suffix, GzSuffix) {
			suffix = suffix[:len(suffix)-len(GzSuffix)]
			compressed = true
		}
		// Require an all-digit (sub)suffix so unrelated siblings (a non-digit
		// suffix, an in-flight ".gz.tmp") are not spliced into the stream.
		seq, perr := strconv.ParseUint(suffix, 10, 64)
		if perr != nil {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			// Raced away between ReadDir and Info; skip it.
			continue
		}
		cur := Segment{
			Path:       filepath.Join(dir, name),
			Seq:        seq,
			Size:       info.Size(),
			Compressed: compressed,
		}
		if prev, ok := bySeq[seq]; ok && (!prev.Compressed || cur.Compressed) {
			// Keep prev unless it is compressed and cur is raw (raw wins).
			continue
		}
		bySeq[seq] = cur
	}

	segs := make([]Segment, 0, len(bySeq))
	for _, s := range bySeq {
		segs = append(segs, s)
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
		data, rerr := readSegment(p)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				// Pruned or rotated away between listing and read; skip.
				continue
			}
			return nil, rerr
		}
		out = append(out, data...)
	}
	return out, nil
}

// readSegment reads one segment fully into memory, decompressing transparently
// when the path is a ".gz" closed segment. An absent path returns an
// os.IsNotExist error so ReadAll can skip a segment pruned between listing and
// read.
func readSegment(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var r io.Reader = f
	if strings.HasSuffix(path, GzSuffix) {
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return nil, fmt.Errorf("open gzip segment %q: %w", path, gerr)
		}
		defer func() { _ = gz.Close() }()
		r = gz
	}
	data, rerr := io.ReadAll(r)
	if rerr != nil {
		return nil, fmt.Errorf("read capture segment %q: %w", path, rerr)
	}
	return data, nil
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

	var r io.Reader = f
	if strings.HasSuffix(path, GzSuffix) {
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return fmt.Errorf("open gzip segment %q: %w", path, gerr)
		}
		defer func() { _ = gz.Close() }()
		r = gz
	}
	if _, cerr := io.Copy(w, r); cerr != nil {
		return fmt.Errorf("read capture segment %q: %w", path, cerr)
	}
	return nil
}

// CompressSegment gzip-compresses the raw closed segment at rawPath to
// rawPath+GzSuffix and removes the raw file, returning the compressed path.
//
// It writes to a temporary sibling and renames atomically so a concurrent
// reader never observes a partial ".gz", and removes the raw segment only
// after the compressed file is in place — so at least one complete
// representation of the segment is on disk at every instant. A non-nil error
// with a non-empty returned path means the compressed copy landed but the raw
// file could not be removed (harmless: ClosedSegments dedups, preferring raw).
func CompressSegment(rawPath string) (string, error) {
	gzPath := rawPath + GzSuffix
	tmp := gzPath + ".tmp"

	in, err := os.Open(rawPath)
	if err != nil {
		return "", fmt.Errorf("open raw segment %q: %w", rawPath, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create compressed segment %q: %w", tmp, err)
	}
	gz := gzip.NewWriter(out)
	if _, cerr := io.Copy(gz, in); cerr != nil {
		_ = gz.Close()
		_ = out.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("gzip segment %q: %w", rawPath, cerr)
	}
	if cerr := gz.Close(); cerr != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("flush gzip segment %q: %w", tmp, cerr)
	}
	if cerr := out.Close(); cerr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close compressed segment %q: %w", tmp, cerr)
	}
	if rerr := os.Rename(tmp, gzPath); rerr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename compressed segment %q to %q: %w", tmp, gzPath, rerr)
	}
	if rerr := os.Remove(rawPath); rerr != nil && !os.IsNotExist(rerr) {
		return gzPath, fmt.Errorf("remove raw segment %q after compress: %w", rawPath, rerr)
	}
	return gzPath, nil
}
