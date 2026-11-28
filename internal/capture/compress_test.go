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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	pcapture "github.com/eminwux/sbsh/pkg/capture"
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

func TestCompressSegment_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	raw := pcapture.SegmentPath(canonical, 0)
	// Repetitive payload so gzip actually shrinks it — also exercises a
	// payload larger than one gzip block.
	original := bytes.Repeat([]byte("the quick brown fox jumped over\n"), 4096)
	writeFile(t, raw, original)

	gzPath, err := CompressSegment(raw)
	if err != nil {
		t.Fatalf("CompressSegment: %v", err)
	}
	if want := raw + pcapture.GzSuffix; gzPath != want {
		t.Fatalf("gzPath = %q, want %q", gzPath, want)
	}
	// Raw is removed; only the compressed copy remains.
	if _, serr := os.Stat(raw); !os.IsNotExist(serr) {
		t.Fatalf("raw segment still present after compress: %v", serr)
	}
	info, err := os.Stat(gzPath)
	if err != nil {
		t.Fatalf("stat %q: %v", gzPath, err)
	}
	if info.Size() >= int64(len(original)) {
		t.Fatalf("compressed size %d not smaller than original %d", info.Size(), len(original))
	}
	// No in-flight temp file is left behind.
	if _, serr := os.Stat(gzPath + ".tmp"); !os.IsNotExist(serr) {
		t.Fatalf("temp file lingered: %v", serr)
	}

	// Round-trip: the compressed segment reads back as the original bytes.
	got, err := pcapture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(original))
	}
}
