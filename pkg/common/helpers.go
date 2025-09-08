package common

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

// generate short random ID (8 hex chars)
func RandomID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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
