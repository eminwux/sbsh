package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

func RuntimeBaseSessions() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sbsh", "run", "sessions"), nil
}

func RuntimeBaseSupervisor() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sbsh", "run", "supervisors"), nil
}

func ParseLevel(lvl string) slog.Level {
	switch lvl {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default:
		// default if unknown
		return slog.LevelInfo
	}
}

func WriteMetadata(ctx context.Context, spec any, dir string) error {

	dst := filepath.Join(dir, "metadata.json")
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", dir, err)
	}
	data = append(data, '\n')

	// Allow cancellation before disk work
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := atomicWriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// atomicWriteFile writes to a temp file in the same dir, fsyncs, then renames.
func atomicWriteFile(dst string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dst)

	f, err := os.CreateTemp(dir, ".meta-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp) // safe if already renamed
	}()

	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Sync(); err != nil { // flush file
		return fmt.Errorf("fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	// Best-effort dir sync after rename for extra safety (Linux/Unix).
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
