package supervisorrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"sbsh/pkg/common"
)

func (s *SupervisorRunnerExec) CreateMetadata() error {

	if err := os.MkdirAll(s.getSupervisorsDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	return common.WriteMetadata(s.ctx, s.spec, s.getSupervisorsDir())
}

func (s *SupervisorRunnerExec) getSupervisorsDir() string {
	return filepath.Join(s.runPath, string(s.id))
}
