package supervisorrunner

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
)

func (sr *SupervisorRunnerExec) OpenSocketCtrl() error {

	sr.supervisorSocketCtrl = filepath.Join(sr.getSupervisorsDir(), "ctrl.sock")
	slog.Debug(fmt.Sprintf("[supervisor] CTRL socket: %s", sr.supervisorSocketCtrl))

	// remove stale socket if it exists
	if _, err := os.Stat(sr.supervisorSocketCtrl); err == nil {
		_ = os.Remove(sr.supervisorSocketCtrl)
	}
	ln, err := net.Listen("unix", sr.supervisorSocketCtrl)
	if err != nil {
		log.Fatal(err)
	}

	sr.lnCtrl = ln

	return nil
}
