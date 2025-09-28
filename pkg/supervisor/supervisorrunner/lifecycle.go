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

func (s *SupervisorRunnerExec) StartSupervisor(ctx context.Context, evCh chan<- SupervisorRunnerEvent, session *sessionstore.SupervisedSession) error {
	s.events = evCh
	s.session = session

	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(session.Command, session.CommandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull

	cmd.Env = append(session.Env, env.KV(env.SUP_SOCKET, s.supervisorSocketCtrl))

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
		trySendEvent(s.events, SupervisorRunnerEvent{ID: api.ID(session.Id), Type: EvCmdExited, Err: err, When: time.Now()})
	}()

	if err := s.dialSessionCtrlSocket(); err != nil {
		return err
	}

	if err := s.attachAndForwardResize(); err != nil {
		return err
	}

	if err := s.attachIOSocket(); err != nil {
		return err
	}

	return nil
}

func (s *SupervisorRunnerExec) Close(reason error) error {
	// remove sockets and dir
	if err := os.Remove(s.supervisorSocketCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] couldn't remove Ctrl socket '%s': %v\r\n", s.supervisorSocketCtrl, err))
	}

	if err := os.RemoveAll(filepath.Dir(s.supervisorSocketCtrl)); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] couldn't remove socket Directory '%s': %v\r\n", s.supervisorSocketCtrl, err))
	}
	s.toExitShell()
	return nil
}

func (s *SupervisorRunnerExec) WaitClose(reason error) error {
	return nil
}

func (s *SupervisorRunnerExec) Resize(args api.ResizeArgs) {
	// No-op
}

func (s *SupervisorRunnerExec) Detach() error {

	// tell session socket control to detach

	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()

	if err := s.sessionClient.Detach(ctx); err != nil {
		return fmt.Errorf("status failed: %w", err)
	}

	// wait for confirmation from the session
	// call close
	// change to cooked mode again
	// quit supervisor leaving session open
	return nil
}

func (s *SupervisorRunnerExec) SetCurrentSession(id api.ID) error {
	// Initial terminal mode (bash passthrough)
	if err := s.toBashUIMode(); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] initial raw mode failed: %v", err))
	}
	return nil
}
