package supervisorrunner

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"syscall"
	"time"
)

const (
	deleteSupervisorDir bool = false
)

func (sr *SupervisorRunnerExec) StartSessionCmd(session *api.SupervisedSession) error {

	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(session.Command, session.CommandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull

	cmd.Env = session.Env

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w :%v", errdefs.ErrSessionCmdStart, err)
	}

	// you can return cmd.Process.Pid to record in meta.json
	session.Pid = cmd.Process.Pid

	slog.Debug(fmt.Sprintf("[supervisor] session %s process %d has started\r\n", session.Id, session.Pid))

	// IMPORTANT: reap it in the background so it never zombifies
	go func() {
		_ = cmd.Wait()
		slog.Debug(fmt.Sprintf("[supervisor] session %s process has exited\r\n", session.Id))
		err := fmt.Errorf("session %s process has exited", session.Id)
		trySendEvent(sr.events, SupervisorRunnerEvent{ID: api.ID(session.Id), Type: EvCmdExited, Err: err, When: time.Now()})
	}()

	return nil
}

func (sr *SupervisorRunnerExec) Attach(session *api.SupervisedSession) error {
	sr.session = session
	var err error
	if err := sr.dialSessionCtrlSocket(); err != nil {
		return err
	}

	conn, err := sr.attach()
	if err != nil {
		return err
	}

	if err := sr.forwardResize(); err != nil {
		return err
	}

	if err := sr.attachIOSocket(conn); err != nil {
		return err
	}

	if err := sr.initTerminal(conn); err != nil {
		return err
	}

	return nil
}

func (sr *SupervisorRunnerExec) Close(reason error) error {
	sr.ctxCancel()
	// remove sockets and dir
	if err := os.Remove(sr.supervisorSocketCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] couldn't remove Ctrl socket '%s': %v\r\n", sr.supervisorSocketCtrl, err))
	}

	if deleteSupervisorDir {
		if err := os.RemoveAll(filepath.Dir(sr.supervisorSocketCtrl)); err != nil {
			slog.Debug(fmt.Sprintf("[supervisor] couldn't remove socket Directory '%s': %v\r\n", sr.supervisorSocketCtrl, err))
		}
	}

	sr.toExitShell()
	return nil
}

func (sr *SupervisorRunnerExec) WaitClose(reason error) error {
	return nil
}

func (sr *SupervisorRunnerExec) Resize(args api.ResizeArgs) {
	// No-op
}

func (sr *SupervisorRunnerExec) Detach() error {
	if err := sr.sessionClient.Detach(sr.ctx, &sr.id); err != nil {
		return err
	}

	return nil
}
