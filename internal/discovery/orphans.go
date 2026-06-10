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
// terminal directory still hosts a live control socket. Mirrors the probe
// in terminalrunner.OpenSocketCtrl: a successful dial means a live peer is
// serving the path; ENOENT/ECONNREFUSED/timeout means nobody is accepting.
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
	for _, dir := range orphans {
		if hasLiveSocket(ctx, dir) {
			logger.WarnContext(
				ctx,
				"pruneOrphanTerminalDirs: skipping orphan dir with live socket",
				"dir", dir,
			)
			if w != nil {
				fmt.Fprintf(w, "Skipped orphan terminal directory %s (live socket)\n", dir)
			}
			continue
		}
		if errRemove := os.RemoveAll(dir); errRemove != nil {
			logger.ErrorContext(
				ctx,
				"pruneOrphanTerminalDirs: failed to remove orphan dir",
				"dir", dir,
				"error", errRemove,
			)
			return fmt.Errorf("prune orphan terminal dir %s: %w", dir, errRemove)
		}
		logger.InfoContext(ctx, "pruneOrphanTerminalDirs: orphan terminal dir removed", "dir", dir)
		if w != nil {
			fmt.Fprintf(w, "Pruned orphan terminal directory %s (missing or corrupt metadata.json)\n", dir)
		}
	}
	return nil
}

// hasLiveSocket reports whether any unix socket directly inside dir accepts
// a connection. Best-effort: a terminal whose control socket was placed
// outside its run dir (custom Spec.SocketFile) is not detected, but the
// default layout (runPath/terminals/<id>/socket) is covered.
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
	if len(orphans) == 0 {
		return 0, nil
	}
	plural := "y"
	if len(orphans) > 1 {
		plural = "ies"
	}
	fmt.Fprintf(
		w,
		"warning: %d terminal director%s with missing or corrupt metadata.json; run 'sb prune terminals' to clean up\n",
		len(orphans), plural,
	)
	return len(orphans), nil
}
