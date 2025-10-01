package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"sort"
	"strings"
	"text/tabwriter"
)

// ScanAndPrintSessions finds all metadata.json under runPath/sessions/*,
// unmarshals them into api.SessionSpec, and prints a table to w.
func ScanAndPrintSessions(ctx context.Context, runPath string, w io.Writer) error {
	sessions, err := scanSessions(ctx, runPath)
	if err != nil {
		return err
	}
	return printSessions(w, sessions)
}

func scanSessions(ctx context.Context, runPath string) ([]api.SessionMetadata, error) {
	pattern := filepath.Join(runPath, "sessions", "*", "metadata.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	out := make([]api.SessionMetadata, 0, len(paths))
	for _, p := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		var s api.SessionMetadata
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("decode %s: %w", p, err)
		}
		out = append(out, s)
	}

	// Optional: stable order by ID (fallback to Name if ID empty)
	sort.Slice(out, func(i, j int) bool {
		idi, idj := sessionID(out[i]), sessionID(out[j])
		if idi != idj {
			return idi < idj
		}
		return sessionName(out[i]) < sessionName(out[j])
	})

	return out, nil
}

func printSessions(w io.Writer, sessions []api.SessionMetadata) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(sessions) == 0 {
		fmt.Fprintf(tw, "no active sessions found\n")
		return tw.Flush()
	}

	fmt.Fprintln(tw, "ID\tNAME\tCMD\tLABELS")
	for _, s := range sessions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			sessionID(s),
			sessionName(s),
			sessionCmd(s),
			joinLabels(sessionLabels(s)),
		)
	}
	return tw.Flush()
}

// --- helpers (adjust to your api.SessionSpec fields if needed) ---

func sessionID(s api.SessionMetadata) string {
	// If your type uses Id instead of ID, change to: return s.Id
	return string(s.Spec.ID)
}

func sessionName(s api.SessionMetadata) string {
	return s.Spec.Name
}

func sessionCmd(s api.SessionMetadata) string {
	parts := make([]string, 0, 1+len(s.Spec.CommandArgs))
	if s.Spec.Command != "" {
		parts = append(parts, s.Spec.Command)
	}
	parts = append(parts, s.Spec.CommandArgs...)
	return strings.Join(parts, " ")
}

func sessionLabels(s api.SessionMetadata) map[string]string {
	if len(s.Spec.Labels) != 0 {
		return s.Spec.Labels
	}
	return map[string]string{}
}

func joinLabels(m map[string]string) string {
	if len(m) == 0 {
		return "none"
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	parts := make([]string, 0, len(ks))
	for _, k := range ks {
		parts = append(parts, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return strings.Join(parts, ",")
}
