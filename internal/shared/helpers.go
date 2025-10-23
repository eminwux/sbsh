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

package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteMetadata(_ context.Context, metadata any, dir string) error {
	dst := filepath.Join(dir, "metadata.json")
	marshaled, marshalErr := json.MarshalIndent(metadata, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal %s: %w", dir, marshalErr)
	}
	marshaled = append(marshaled, '\n') // gocritic: assign result to same slice

	const filePerm = 0o644 // mnd: magic number
	if writeErr := atomicWriteFile(dst, marshaled, filePerm); writeErr != nil {
		return fmt.Errorf("write %s: %w", dst, writeErr)
	}
	return nil
}

// atomicWriteFile writes to a temp file in the same dir, fsyncs, then renames.
func atomicWriteFile(dst string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dst)

	f, createErr := os.CreateTemp(dir, ".meta-*.tmp")
	if createErr != nil {
		return createErr
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp) // safe if already renamed
	}()

	if chmodErr := f.Chmod(mode); chmodErr != nil {
		return fmt.Errorf("chmod: %w", chmodErr)
	}
	if _, writeErr := f.Write(data); writeErr != nil {
		return fmt.Errorf("write: %w", writeErr)
	}
	if syncErr := f.Sync(); syncErr != nil { // flush file
		return fmt.Errorf("fsync: %w", syncErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		return fmt.Errorf("close: %w", closeErr)
	}

	// Best-effort dir sync after rename for extra safety (Linux/Unix).
	if renameErr := os.Rename(tmp, dst); renameErr != nil {
		return fmt.Errorf("rename: %w", renameErr)
	}
	if d, openErr := os.Open(dir); openErr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
