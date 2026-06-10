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
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	pkgdiscovery "github.com/eminwux/sbsh/pkg/discovery"
)

// liveSocketProbeTimeout bounds the dial used to decide whether an orphan
// terminal or client directory still hosts a live control socket. Mirrors
// the probe in terminalrunner.OpenSocketCtrl: a successful dial means a live
// peer is serving the path; ENOENT/ECONNREFUSED/timeout means nobody is
// accepting.
const liveSocketProbeTimeout = 100 * time.Millisecond

// pruneOrphanTerminalDirs removes terminal directories whose metadata.json
// is missing or unparseable. ScanTerminals can never return such
// directories, so the metadata-driven prune in ScanAndPruneTerminals never
// reaps them and they leak on disk indefinitely (see #391). A directory
// hosting a live unix socket is never removed: corrupt metadata does not
// prove the terminal process is gone, and a live socket proves it is not.
func pruneOrphanTerminalDirs(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) error {
	orphans, err := pkgdiscovery.OrphanTerminalDirs(ctx, logger, runPath)
	if err != nil {
		return err
	}
	return pruneOrphanDirs(ctx, logger, w, orphans, "terminal")
}

// pruneOrphanClientDirs removes client directories whose metadata.json is
// missing or unparseable — the clients-side counterpart of
// pruneOrphanTerminalDirs (see #422). The live-socket guard covers the
// client control socket (clients/<id>/client.sock — see pkg/attach).
func pruneOrphanClientDirs(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) error {
	orphans, err := pkgdiscovery.OrphanClientDirs(ctx, logger, runPath)
	if err != nil {
		return err
	}
	return pruneOrphanDirs(ctx, logger, w, orphans, "client")
}

// pruneOrphanDirs removes the given orphan directories, skipping any that
// still host a live unix socket. noun ("terminal", "client") labels logs and
// the per-directory lines written to w.
func pruneOrphanDirs(ctx context.Context, logger *slog.Logger, w io.Writer, orphans []string, noun string) error {
	for _, dir := range orphans {
		if hasLiveSocket(ctx, dir) {
			logger.WarnContext(
				ctx,
				"pruneOrphanDirs: skipping orphan dir with live socket",
				"kind", noun,
				"dir", dir,
			)
			if w != nil {
				fmt.Fprintf(w, "Skipped orphan %s directory %s (live socket)\n", noun, dir)
			}
			continue
		}
		if errRemove := os.RemoveAll(dir); errRemove != nil {
			logger.ErrorContext(
				ctx,
				"pruneOrphanDirs: failed to remove orphan dir",
				"kind", noun,
				"dir", dir,
				"error", errRemove,
			)
			return fmt.Errorf("prune orphan %s dir %s: %w", noun, dir, errRemove)
		}
		logger.InfoContext(ctx, "pruneOrphanDirs: orphan dir removed", "kind", noun, "dir", dir)
		if w != nil {
			fmt.Fprintf(w, "Pruned orphan %s directory %s (missing or corrupt metadata.json)\n", noun, dir)
		}
	}
	return nil
}

// hasLiveSocket reports whether any unix socket directly inside dir accepts
// a connection. Best-effort: a terminal whose control socket was placed
// outside its run dir (custom Spec.SocketFile) is not detected, but the
// default layouts (runPath/terminals/<id>/socket,
// runPath/clients/<id>/client.sock) are covered.
func hasLiveSocket(ctx context.Context, dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	dialer := net.Dialer{Timeout: liveSocketProbeTimeout}
	for _, e := range entries {
		if e.Type()&fs.ModeSocket == 0 {
			continue
		}
		conn, errDial := dialer.DialContext(ctx, "unix", filepath.Join(dir, e.Name()))
		if errDial == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// ReportOrphanTerminalDirs writes a one-line warning to w when directories
// with missing or unparseable metadata.json exist under runPath/terminals,
// and returns the orphan count. It gives `sb get terminals` a non-verbose
// signal that invisible directories need cleanup — without it the only
// trace is a Warn-level log hidden behind --verbose. Best-effort: callers
// surface the count, not the listing; `sb prune terminals` does the reaping.
func ReportOrphanTerminalDirs(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) (int, error) {
	orphans, err := pkgdiscovery.OrphanTerminalDirs(ctx, logger, runPath)
	if err != nil {
		return 0, err
	}
	return reportOrphanDirs(w, orphans, "terminal", "sb prune terminals"), nil
}

// ReportOrphanClientDirs is the clients-side counterpart of
// ReportOrphanTerminalDirs: a one-line warning channel for `sb get clients`
// when orphan client directories exist; `sb prune clients` does the reaping.
func ReportOrphanClientDirs(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) (int, error) {
	orphans, err := pkgdiscovery.OrphanClientDirs(ctx, logger, runPath)
	if err != nil {
		return 0, err
	}
	return reportOrphanDirs(w, orphans, "client", "sb prune clients"), nil
}

// reportOrphanDirs writes the one-line orphan warning to w and returns the
// orphan count. Silent when there are no orphans.
func reportOrphanDirs(w io.Writer, orphans []string, noun, pruneCmd string) int {
	if len(orphans) == 0 {
		return 0
	}
	plural := "y"
	if len(orphans) > 1 {
		plural = "ies"
	}
	fmt.Fprintf(
		w,
		"warning: %d %s director%s with missing or corrupt metadata.json; run '%s' to clean up\n",
		len(orphans), noun, plural, pruneCmd,
	)
	return len(orphans)
}
