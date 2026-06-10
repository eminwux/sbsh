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

// orphanGracePeriod shields a just-created terminal or client directory from
// being classified as orphaned. The runner MkdirAlls the directory before
// the first atomic metadata.json write lands, so a scan racing that window
// would otherwise see a metadata-less directory that is about to become a
// live entity and let prune remove it mid-startup.
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
	return orphanDirs[api.TerminalDoc](ctx, logger, runPath, defaults.TerminalsRunPath, "OrphanTerminalDirs", "terminal")
}

// OrphanClientDirs returns the directories under runPath/clients whose
// metadata.json is missing or unparseable. The same leak exists for clients
// as for terminals: ScanClients shares the metadata glob and the
// skip-on-decode-failure design, so such directories can never be listed or
// reaped without this enumeration. Classification rules (unreadable files,
// grace window) match OrphanTerminalDirs.
func OrphanClientDirs(ctx context.Context, logger *slog.Logger, runPath string) ([]string, error) {
	return orphanDirs[api.ClientDoc](ctx, logger, runPath, defaults.ClientsRunPath, "OrphanClientDirs", "client")
}

// orphanDirs enumerates the directories under runPath/subDir whose
// metadata.json is missing or does not decode into T. See the exported
// wrappers for the classification contract.
func orphanDirs[T any](
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	subDir string,
	logPrefix string,
	noun string,
) ([]string, error) {
	root := filepath.Join(runPath, subDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		logger.ErrorContext(ctx, fmt.Sprintf("%s: failed to read %ss root", logPrefix, noun), "dir", root, "error", err)
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
			var doc T
			if json.Unmarshal(b, &doc) == nil {
				continue
			}
		} else if !errors.Is(errRead, fs.ErrNotExist) {
			logger.WarnContext(
				ctx,
				fmt.Sprintf("%s: metadata.json unreadable, not classifying as orphan", logPrefix),
				"dir", dir,
				"error", errRead,
			)
			continue
		}
		if info, errInfo := e.Info(); errInfo == nil && time.Since(info.ModTime()) < orphanGracePeriod {
			logger.DebugContext(ctx, fmt.Sprintf("%s: skipping recently modified dir", logPrefix), "dir", dir)
			continue
		}
		logger.WarnContext(
			ctx,
			fmt.Sprintf("%s: %s dir has missing or unparseable metadata.json", logPrefix, noun),
			"dir", dir,
		)
		orphans = append(orphans, dir)
	}
	return orphans, nil
}
