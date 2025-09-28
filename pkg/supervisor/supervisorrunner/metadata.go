package supervisorrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"sbsh/pkg/common"
)

func (sr *SupervisorRunnerExec) CreateMetadata() error {

	if err := os.MkdirAll(sr.getSupervisorsDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	return common.WriteMetadata(sr.ctx, sr.spec, sr.getSupervisorsDir())
}

func (sr *SupervisorRunnerExec) getSupervisorsDir() string {
	return filepath.Join(sr.runPath, string(sr.id))
}
