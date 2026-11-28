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

package capture

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
// compressed: "<canonical>.NNNNNN.gz".
const GzSuffix = ".gz"

// SegmentPath returns the deterministic closed-segment path for seq, a sibling
// of canonical: "<canonical>.NNNNNN".
func SegmentPath(canonical string, seq uint64) string {
	return fmt.Sprintf("%s.%0*d", canonical, seqWidth, seq)
}

// Segment is one segment of a capture on disk.
type Segment struct {
	// Path is the path the segment was listed under: the canonical path for
	// the live segment, "<canonical>.NNNNNN[.gz]" for a closed one.
	Path string
	// Seq is the segment's sequence number. For the live segment it is the
	// sequence the segment keeps when it is rotated (see NextSeq).
	Seq uint64
	// Size is the on-disk size at listing time: for a compressed segment this
	// is the gzip size.
	Size int64
	// Compressed is true when Path is a ".gz" segment that readers must
	// decompress.
	Compressed bool
	// Live is true for the canonical (live, append-only, raw) segment.
	Live bool
}

// ClosedSegments returns the closed segments of the capture at canonical,
// ordered oldest-first (ascending sequence). It does not include the live
// segment. A missing directory yields an empty slice and no error.
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

// NextSeq returns the sequence number the live segment takes when it is next
// rotated: one past the highest existing closed segment, or 0 when none exist.
// Lets a writer resume a deterministic, gap-free ordering across restarts.
func NextSeq(canonical string) (uint64, error) {
	segs, err := ClosedSegments(canonical)
	if err != nil {
		return 0, err
	}
	return nextSeqOf(segs), nil
}

func nextSeqOf(closed []Segment) uint64 {
	if len(closed) == 0 {
		return 0
	}
	return closed[len(closed)-1].Seq + 1
}

// Segments returns the full reassembly order of the capture at canonical:
// closed segments oldest-first, then the live segment (if it exists on disk)
// with Live set and Seq equal to NextSeq — the sequence it keeps when it is
// rotated. A capture that was never written yields an empty slice and no
// error.
func Segments(canonical string) ([]Segment, error) {
	segs, err := ClosedSegments(canonical)
	if err != nil {
		return nil, err
	}
	info, serr := os.Stat(canonical)
	switch {
	case serr == nil:
		segs = append(segs, Segment{
			Path: canonical,
			Seq:  nextSeqOf(segs),
			Size: info.Size(),
			Live: true,
		})
	case !os.IsNotExist(serr):
		return nil, fmt.Errorf("stat capture file %q: %w", canonical, serr)
	}
	return segs, nil
}

// siblingPath returns the alternate on-disk representation of a closed
// segment: the ".gz" form for a raw path, or the raw form for a ".gz" path.
func siblingPath(path string) string {
	if strings.HasSuffix(path, GzSuffix) {
		return strings.TrimSuffix(path, GzSuffix)
	}
	return path + GzSuffix
}

// openClosed opens a closed segment for reading, recovering from the raw↔".gz"
// rename race: a segment listed as raw can be swapped to ".gz" by a concurrent
// compression (or vice-versa) in the window before the reader opens it. On
// ENOENT for the listed path it re-resolves the sibling representation — the
// same retained segment under its other name — rather than dropping
// still-retained history (issue #316). It returns the opened file and whether
// that file is gzip-compressed (dispatch follows the path actually opened, not
// the one requested). Only when *neither* representation exists is the
// original not-exist error returned.
func openClosed(path string) (*os.File, bool, error) {
	f, err := os.Open(path)
	if err == nil {
		return f, strings.HasSuffix(path, GzSuffix), nil
	}
	if !os.IsNotExist(err) {
		return nil, false, err
	}
	sib := siblingPath(path)
	sf, serr := os.Open(sib)
	if serr != nil {
		if os.IsNotExist(serr) {
			return nil, false, err
		}
		return nil, false, serr
	}
	return sf, strings.HasSuffix(sib, GzSuffix), nil
}

// OpenStored opens a listed segment's bytes as stored on disk, without
// decompressing, and reports whether the opened file is gzip-compressed.
//
// It recovers from the renames rotation performs between listing and open:
// a closed segment is found under its raw or ".gz" name, whichever exists, so
// compressed may differ from s.Compressed. A live segment that was rotated
// away is found under its closed name (SegmentPath(s.Path, s.Seq), raw or
// ".gz"). Note that once the live segment has rotated, the canonical path
// holds the next live segment; a caller holding offsets into a live segment
// detects that by re-listing (a closed segment with s.Seq appears). When no
// representation exists — the segment was pruned — the error satisfies
// errors.Is(err, fs.ErrNotExist).
func OpenStored(s Segment) (*os.File, bool, error) {
	if !s.Live {
		return openClosed(s.Path)
	}
	f, err := os.Open(s.Path)
	if err == nil {
		return f, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, false, err
	}
	cf, compressed, cerr := openClosed(SegmentPath(s.Path, s.Seq))
	if cerr != nil {
		if errors.Is(cerr, fs.ErrNotExist) {
			return nil, false, err
		}
		return nil, false, cerr
	}
	return cf, compressed, nil
}

// Open opens a listed segment as a decompressed byte stream, with the same
// rename recovery as OpenStored. Closing the returned reader closes the file.
func Open(s Segment) (io.ReadCloser, error) {
	f, compressed, err := OpenStored(s)
	if err != nil {
		return nil, err
	}
	if !compressed {
		return f, nil
	}
	gz, gerr := gzip.NewReader(f)
	if gerr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open gzip segment %q: %w", f.Name(), gerr)
	}
	return &gzipReadCloser{gz: gz, f: f}, nil
}

type gzipReadCloser struct {
	gz *gzip.Reader
	f  *os.File
}

func (g *gzipReadCloser) Read(p []byte) (int, error) { return g.gz.Read(p) }

func (g *gzipReadCloser) Close() error {
	gerr := g.gz.Close()
	if ferr := g.f.Close(); ferr != nil {
		return ferr
	}
	return gerr
}

// ReadAll reassembles the entire capture — closed segments oldest-first, then
// the live segment — into one decompressed byte slice. A capture that was
// never written yields a nil slice and no error; a segment pruned between
// listing and read is skipped.
func ReadAll(canonical string) ([]byte, error) {
	var out []byte
	err := each(canonical, func(s Segment, r io.Reader) error {
		data, rerr := io.ReadAll(r)
		if rerr != nil {
			return fmt.Errorf("read capture segment %q: %w", s.Path, rerr)
		}
		out = append(out, data...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Dump streams the reassembled, decompressed capture to w in order: closed
// segments oldest-first, then the live segment. A capture that was never
// written produces no output and no error.
func Dump(canonical string, w io.Writer) error {
	return each(canonical, func(s Segment, r io.Reader) error {
		if _, cerr := io.Copy(w, r); cerr != nil {
			return fmt.Errorf("read capture segment %q: %w", s.Path, cerr)
		}
		return nil
	})
}

// each opens every listed segment of canonical in order and hands its
// decompressed stream to fn, skipping segments pruned after listing.
func each(canonical string, fn func(Segment, io.Reader) error) error {
	segs, err := Segments(canonical)
	if err != nil {
		return err
	}
	for _, s := range segs {
		r, oerr := Open(s)
		if oerr != nil {
			if errors.Is(oerr, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("open capture segment %q: %w", s.Path, oerr)
		}
		ferr := fn(s, r)
		_ = r.Close()
		if ferr != nil {
			return ferr
		}
	}
	return nil
}
