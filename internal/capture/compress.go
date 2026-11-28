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
	"fmt"
	"io"
	"os"

	pcapture "github.com/eminwux/sbsh/pkg/capture"
)

// CompressSegment gzip-compresses the raw closed segment at rawPath to
// rawPath+".gz" and removes the raw file, returning the compressed path.
//
// It writes to a temporary sibling and renames atomically so a concurrent
// reader never observes a partial ".gz", and removes the raw segment only
// after the compressed file is in place — so at least one complete
// representation of the segment is on disk at every instant. A non-nil error
// with a non-empty returned path means the compressed copy landed but the raw
// file could not be removed (harmless: pcapture.ClosedSegments dedups, preferring raw).
func CompressSegment(rawPath string) (string, error) {
	gzPath := rawPath + pcapture.GzSuffix
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
