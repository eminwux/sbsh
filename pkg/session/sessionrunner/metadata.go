package sessionrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
)

func (sr *SessionRunnerExec) CreateMetadata() error {

	if err := os.MkdirAll(sr.getSessionDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	return sr.updateMetadata()
}

func (sr *SessionRunnerExec) getSessionDir() string {
	return filepath.Join(sr.runPath, string(sr.id))
}

func (sr *SessionRunnerExec) updateMetadata() error {
	metadata := api.SessionMetadata{
		Spec:   &sr.spec,
		Status: &sr.status,
	}
	return common.WriteMetadata(sr.ctx, metadata, sr.getSessionDir())
}
