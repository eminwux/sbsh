package supervisorrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/env"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/supervisor/sessionstore"
	"syscall"
	"time"
)

const deleteSupervisorDir bool = false

func (sr *SupervisorRunnerExec) StartSupervisor(ctx context.Context, evCh chan<- SupervisorRunnerEvent, session *sessionstore.SupervisedSession) error {
	sr.events = evCh
	sr.session = session

	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(session.Command, session.CommandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull

	cmd.Env = append(session.Env, env.KV(env.SUP_SOCKET, sr.supervisorSocketCtrl))

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w:%w", errdefs.ErrSessionCmdStart, err)
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

	if err := sr.dialSessionCtrlSocket(); err != nil {
		return err
	}

	if err := sr.attachAndForwardResize(); err != nil {
		return err
	}

	if err := sr.attachIOSocket(); err != nil {
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

	// tell session socket control to detach

	ctx, cancel := context.WithTimeout(sr.ctx, 3*time.Second)
	defer cancel()

	if err := sr.sessionClient.Detach(ctx); err != nil {
		return fmt.Errorf("status failed: %w", err)
	}

	// wait for confirmation from the session
	// call close
	// change to cooked mode again
	// quit supervisor leaving session open
	return nil
}

func (sr *SupervisorRunnerExec) SetCurrentSession(id api.ID) error {
	// Initial terminal mode (bash passthrough)
	if err := sr.toBashUIMode(); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] initial raw mode failed: %v", err))
	}
	return nil
}
