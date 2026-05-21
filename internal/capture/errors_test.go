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
	"io"
	"path/filepath"
	"testing"
)

// TestCompressSegment_OpenRawFails covers the open-raw error branch: a
// nonexistent rawPath returns an error and no compressed path.
func TestCompressSegment_OpenRawFails(t *testing.T) {
	raw := filepath.Join(t.TempDir(), "capture.000000")
	gzPath, err := CompressSegment(raw)
	if err == nil {
		t.Fatal("expected error compressing a nonexistent segment")
	}
	if gzPath != "" {
		t.Fatalf("expected empty path on open failure, got %q", gzPath)
	}
}

// TestReadAll_CorruptGzSegmentErrors covers readSegment's gzip-open error
// branch: a ".gz" segment whose contents are not valid gzip surfaces an error
// rather than silently yielding garbage.
func TestReadAll_CorruptGzSegmentErrors(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	// A .gz segment that is not actually gzip-compressed.
	writeFile(t, SegmentPath(canonical, 0)+GzSuffix, []byte("not gzip data"))

	if _, err := ReadAll(canonical); err == nil {
		t.Fatal("expected error reading a corrupt gzip segment")
	}
}

// TestDump_CorruptGzSegmentErrors mirrors the ReadAll case for dumpOne's
// gzip-open error branch.
func TestDump_CorruptGzSegmentErrors(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	writeFile(t, SegmentPath(canonical, 0)+GzSuffix, []byte("not gzip data"))

	if err := Dump(canonical, io.Discard); err == nil {
		t.Fatal("expected error dumping a corrupt gzip segment")
	}
}

// TestReplay_ReadError covers Replay's error propagation from ReadAll when a
// segment cannot be decoded.
func TestReplay_ReadError(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	writeFile(t, SegmentPath(canonical, 0)+GzSuffix, []byte("not gzip data"))

	if _, err := Replay(canonical); err == nil {
		t.Fatal("expected Replay to surface the decode error")
	}
}
