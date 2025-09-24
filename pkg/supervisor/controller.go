package supervisor

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
	"sbsh/pkg/supervisor/supervisorrunner"
	"time"
)

/* ---------- Controller ---------- */

type SupervisorController struct {
	ctx     context.Context
	exit    chan struct{}
	closed  chan struct{}
	closing chan error

	listenerCtrl net.Listener
}

var newSupervisorRunner = supervisorrunner.NewSupervisorRunnerExec
var sr supervisorrunner.SupervisorRunner

var newSessionStore = sessionstore.NewSessionStoreExec
var ss sessionstore.SessionStore

var ctrlReady chan struct{} = make(chan struct{})
var rpcReadyCh chan error = make(chan error)
var rpcDoneCh chan error = make(chan error)
var closeReqCh chan error = make(chan error, 1)
var eventsCh chan supervisorrunner.SupervisorRunnerEvent = make(chan supervisorrunner.SupervisorRunnerEvent, 32)

// NewSupervisorController wires the manager and the shared event channel from sessions.
func NewSupervisorController(ctx context.Context) api.SupervisorController {
	slog.Debug("[supervisor] New controller is being created\r\n")

	c := &SupervisorController{
		ctx:     ctx,
		closed:  make(chan struct{}),
		closing: make(chan error, 1),
	}
	return c
}

func (s *SupervisorController) WaitReady(ctx context.Context) error {
	select {
	case <-ctrlReady:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (s *SupervisorController) Run() error {
	s.exit = make(chan struct{})
	slog.Debug("[supervisor] Starting controller loop")
	defer slog.Debug("[supervisor] controller stopped\r\n")

	sr = newSupervisorRunner(s.ctx)
	ss = newSessionStore()

	ctrlLn, err := sr.OpenSocketCtrl()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not open control socket: %v", err))
		if err := s.Close(err); err != nil {
			err = fmt.Errorf("%w:%w", ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", ErrOpenSocketCtrl, err)
	}

	s.listenerCtrl = ctrlLn

	rpc := &supervisorrpc.SupervisorControllerRPC{Core: s}
	go sr.StartServer(s.ctx, s.listenerCtrl, rpc, rpcReadyCh, rpcDoneCh)
	// Wait for startup result
	if err := <-rpcReadyCh; err != nil {
		// failed to start — handle and return
		if errC := s.Close(err); errC != nil {
			err = fmt.Errorf("%w:%w:%w", err, ErrOnClose, errC)
		}
		slog.Debug(fmt.Sprintf("failed to start server: %v", err))
		return fmt.Errorf("%w:%w", ErrStartRPCServer, err)
	}

	sessionID := common.RandomID()

	// exe := "/home/inwx/projects/sbsh/sbsh-session"
	args := []string{"run", "--id", sessionID}

	execPath, err := os.Executable()
	if err != nil {
		panic(err)
	}

	sessionSpec := &api.SessionSpec{
		ID:          api.SessionID(sessionID),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     execPath,
		CommandArgs: args,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		SockerCtrl:  "/home/inwx/.sbsh/run/sessions/" + sessionID + "/ctrl.sock",
		SocketIO:    "/home/inwx/.sbsh/run/sessions/" + sessionID + "/io.sock",
	}

	session := sessionstore.NewSupervisedSession(sessionSpec)
	if err := ss.Add(session); err != nil {
		return fmt.Errorf("%w:%w", ErrSessionStore, err)
	}

	if err := sr.StartSupervisor(s.ctx, eventsCh, session); err != nil {
		slog.Debug(fmt.Sprintf("failed to start session: %v", err))
		if err := s.Close(err); err != nil {
			err = fmt.Errorf("%w:%w", ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", ErrStartSession, err)
	}
	close(ctrlReady)

	for {
		select {
		case <-s.ctx.Done():
			var err error
			slog.Debug("[supervisor] parent context channel has been closed\r\n")
			if errC := s.Close(s.ctx.Err()); errC != nil {
				err = fmt.Errorf("%w:%w", ErrOnClose, errC)
			}
			return fmt.Errorf("%w:%w", ErrContextDone, err)

		case ev := <-eventsCh:
			slog.Debug(fmt.Sprintf("[supervisor] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano)))
			s.handleEvent(ev)

		case err := <-rpcDoneCh:
			slog.Debug(fmt.Sprintf("[supervisor] rpc server has failed: %v\r\n", err))
			if err := s.Close(err); err != nil {
				err = fmt.Errorf("%w:%w:%w", err, ErrOnClose, err)
			}
			return fmt.Errorf("%w:%w", ErrRPCServerExited, err)

		case err := <-closeReqCh:
			slog.Debug(fmt.Sprintf("[supervisor] close request received: %v\r\n", err))
			return fmt.Errorf("%w:%w", ErrCloseReq, err)
		}
	}
}

/* ---------- Event handlers ---------- */

func (s *SupervisorController) handleEvent(ev supervisorrunner.SupervisorRunnerEvent) {
	// slog.Debug("[supervisor] session %s event received %d\r\n", ev.ID, ev.Type)
	switch ev.Type {
	case supervisorrunner.EvCmdExited:
		slog.Debug(fmt.Sprintf("[supervisor] session %s EvCmdExited error: %v\r\n", ev.ID, ev.Err))
		s.onClosed(ev.ID, ev.Err)

	case supervisorrunner.EvError:
		slog.Debug(fmt.Sprintf("[supervisor] session %s EvError error: %v\r\n", ev.ID, ev.Err))
		s.onClosed(ev.ID, ev.Err)
	}
}

func (s *SupervisorController) onClosed(_ api.SessionID, err error) {
	s.Close(err)
}

func (s *SupervisorController) SetCurrentSession(id api.SessionID) error {
	if err := ss.SetCurrent(id); err != nil {
		log.Fatalf("failed to set current session: %v", err)
		return err
	}
	return nil

}
func (s *SupervisorController) Close(reason error) error {
	slog.Debug(fmt.Sprintf("[supervisor] Close called: %v\r\n", reason))
	s.closing <- reason
	closeReqCh <- reason
	slog.Debug(fmt.Sprintf("[supervisor] error sent to closeReqCh: %v\r\n", reason))
	sr.Close(reason)
	close(s.closed)

	return nil
}

func (s *SupervisorController) WaitClose() error {

	select {
	case <-s.closed:
		slog.Debug("[supervisor] controller exited\r\n")
		return nil
	case err := <-s.closing:
		slog.Debug(fmt.Sprintf("[supervisor] controller closing: %v\r\n", err))
	}
	return nil
}
