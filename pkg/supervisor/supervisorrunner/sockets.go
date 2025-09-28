package supervisorrunner

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
)

func (s *SupervisorRunnerExec) OpenSocketCtrl() error {

	s.supervisorSocketCtrl = filepath.Join(s.getSupervisorsDir(), "ctrl.sock")
	slog.Debug(fmt.Sprintf("[supervisor] CTRL socket: %s", s.supervisorSocketCtrl))

	// remove stale socket if it exists
	if _, err := os.Stat(s.supervisorSocketCtrl); err == nil {
		_ = os.Remove(s.supervisorSocketCtrl)
	}
	ln, err := net.Listen("unix", s.supervisorSocketCtrl)
	if err != nil {
		log.Fatal(err)
	}

	s.lnCtrl = ln

	return nil
}
