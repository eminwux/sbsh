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

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

// orphanGracePeriod shields a just-created terminal directory from being
// classified as orphaned. The terminal runner MkdirAlls the directory
// before the first atomic metadata.json write lands, so a scan racing that
// window would otherwise see a metadata-less directory that is about to
// become a live terminal and let prune remove it mid-startup.
const orphanGracePeriod = 30 * time.Second

// OrphanTerminalDirs returns the directories under runPath/terminals whose
// metadata.json is missing or unparseable. Such directories are invisible
// to ScanTerminals — the metadata glob never matches a missing file, and
// per-file decode failures are skipped by design (see scanMetadataFiles) —
// so without this enumeration they can never be listed or reaped and leak
// on disk indefinitely.
//
// A metadata.json that exists but cannot be read (e.g. permission denied)
// does not classify the directory as orphaned: an unreadable file cannot
// prove the terminal is gone, and the caller likely lacks the rights to
// remove the directory anyway. Directories modified within the last
// orphanGracePeriod are also skipped — see the constant's doc.
func OrphanTerminalDirs(ctx context.Context, logger *slog.Logger, runPath string) ([]string, error) {
	root := filepath.Join(runPath, defaults.TerminalsRunPath)
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		logger.ErrorContext(ctx, "OrphanTerminalDirs: failed to read terminals root", "dir", root, "error", err)
		return nil, fmt.Errorf("read dir %q: %w", root, err)
	}

	var orphans []string
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		b, errRead := os.ReadFile(filepath.Join(dir, "metadata.json"))
		if errRead == nil {
			var doc api.TerminalDoc
			if json.Unmarshal(b, &doc) == nil {
				continue
			}
		} else if !errors.Is(errRead, fs.ErrNotExist) {
			logger.WarnContext(
				ctx,
				"OrphanTerminalDirs: metadata.json unreadable, not classifying as orphan",
				"dir", dir,
				"error", errRead,
			)
			continue
		}
		if info, errInfo := e.Info(); errInfo == nil && time.Since(info.ModTime()) < orphanGracePeriod {
			logger.DebugContext(ctx, "OrphanTerminalDirs: skipping recently modified dir", "dir", dir)
			continue
		}
		logger.WarnContext(
			ctx,
			"OrphanTerminalDirs: terminal dir has missing or unparseable metadata.json",
			"dir", dir,
		)
		orphans = append(orphans, dir)
	}
	return orphans, nil
}
