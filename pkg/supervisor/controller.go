package supervisor

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/discovery"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/naming"
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
	runPath string
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

func (s *SupervisorController) WaitReady() error {
	select {
	case <-ctrlReady:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (s *SupervisorController) Run(spec *api.SupervisorSpec) error {
	s.exit = make(chan struct{})
	slog.Debug("[supervisor] Starting controller loop")
	defer slog.Debug("[supervisor] controller stopped\r\n")

	s.runPath = spec.RunPath

	sr = newSupervisorRunner(s.ctx, spec, eventsCh)
	ss = newSessionStore()

	err := sr.CreateMetadata()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not write metadata file: %v", err))
		if err := s.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w: %v", errdefs.ErrWriteMetadata, err)
	}

	err = sr.OpenSocketCtrl()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not open control socket: %v", err))
		if err := s.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		close(ctrlReady)
		return fmt.Errorf("%w: %v", errdefs.ErrOpenSocketCtrl, err)
	}

	rpc := &supervisorrpc.SupervisorControllerRPC{Core: s}
	go sr.StartServer(s.ctx, rpc, rpcReadyCh, rpcDoneCh)
	// Wait for startup result
	if err := <-rpcReadyCh; err != nil {
		// failed to start — handle and return
		if errC := s.Close(err); errC != nil {
			err = fmt.Errorf("%v: %w: %v", err, errdefs.ErrOnClose, errC)
		}
		close(ctrlReady)
		slog.Debug(fmt.Sprintf("failed to start server: %v", err))
		return fmt.Errorf("%w: %v", errdefs.ErrStartRPCServer, err)
	}

	var session *api.SupervisedSession
	switch spec.Kind {
	case api.RunNewSession:
		session, err = s.CreateRunNewSession()
		if err != nil {
			if errC := s.Close(err); errC != nil {
				err = fmt.Errorf("%w: %v: %v", err, errdefs.ErrOnClose, errC)
			}
			close(ctrlReady)
			return err
		}

		if err := sr.StartSessionCmd(session); err != nil {
			slog.Debug(fmt.Sprintf("failed to start session cmd: %v", err))
			if errC := s.Close(err); errC != nil {
				err = fmt.Errorf("%v: %v: %v", err, errdefs.ErrOnClose, errC)
			}
			close(ctrlReady)
			return fmt.Errorf("%w: %v", errdefs.ErrStartSessionCmd, err)
		}
	case api.AttachToSession:
		session, err = s.CreateAttachSession(spec)
		if err != nil {
			if errC := s.Close(err); errC != nil {
				err = fmt.Errorf("%w: %v: %v", err, errdefs.ErrOnClose, errC)
			}
			close(ctrlReady)
			return err
		}

	default:
		if err := s.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		close(ctrlReady)
		return errdefs.ErrSupervisorKind
	}

	if err := sr.Attach(session); err != nil {
		slog.Debug(fmt.Sprintf("failed to attach: %v", err))
		if err := s.Close(err); err != nil {
			err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, err)
		}
		close(ctrlReady)
		return fmt.Errorf("%w: %v", errdefs.ErrAttach, err)
	}

	close(ctrlReady)

	for {
		select {
		case <-s.ctx.Done():
			var err error
			slog.Debug("[supervisor] parent context channel has been closed\r\n")
			if errC := s.Close(s.ctx.Err()); errC != nil {
				err = fmt.Errorf("%w: %v", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w: %v", errdefs.ErrContextDone, err)

		case ev := <-eventsCh:
			slog.Debug(fmt.Sprintf("[supervisor] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano)))
			s.handleEvent(ev)

		case err := <-rpcDoneCh:
			slog.Debug(fmt.Sprintf("[supervisor] rpc server has failed: %v\r\n", err))
			if err := s.Close(err); err != nil {
				err = fmt.Errorf("%v:%w:%v", err, errdefs.ErrOnClose, err)
			}
			return fmt.Errorf("%w: %v", errdefs.ErrRPCServerExited, err)

		case err := <-closeReqCh:
			slog.Debug(fmt.Sprintf("[supervisor] close request received: %v\r\n", err))
			return fmt.Errorf("%w: %v", errdefs.ErrCloseReq, err)
		}
	}
}

func (s *SupervisorController) CreateAttachSession(spec *api.SupervisorSpec) (*api.SupervisedSession, error) {

	// read from metadata

	var metadata *api.SessionMetadata
	var err error
	if spec.AttachID != "" {
		metadata, err = discovery.FindSessionByID(s.ctx, spec.RunPath, string(spec.AttachID))
		if err != nil {
			return nil, fmt.Errorf("coult not find session by ID")
		}

	} else if spec.AttachName != "" {
		metadata, err = discovery.FindSessionByName(s.ctx, spec.RunPath, spec.AttachName)
		if err != nil {
			return nil, fmt.Errorf("coult not find session by Name")
		}
	}
	session := sessionstore.NewSupervisedSession(metadata.Spec)
	if err := ss.Add(session); err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrSessionStore, err)
	}

	return session, nil
}

func (s *SupervisorController) CreateRunNewSession() (*api.SupervisedSession, error) {

	sessionID := naming.RandomID()
	sessionName := naming.RandomSessionName()

	// exe := "/home/inwx/projects/sbsh/sbsh-session"
	args := []string{"run", "--id", sessionID, "--name", sessionName}

	execPath, err := os.Executable()
	if err != nil {

		return nil, fmt.Errorf("%w: %v", errdefs.ErrStartCmd, err)
	}

	sessionSpec := &api.SessionSpec{
		ID:          api.ID(sessionID),
		Kind:        api.SessLocal,
		Name:        sessionName,
		Command:     execPath,
		CommandArgs: args,
		Env:         os.Environ(),
		LogFilename: s.runPath + "/sessions/" + sessionID + "/log",
		SockerCtrl:  s.runPath + "/sessions/" + sessionID + "/ctrl.sock",
		SocketIO:    s.runPath + "/sessions/" + sessionID + "/io.sock",
	}

	session := sessionstore.NewSupervisedSession(sessionSpec)
	if err := ss.Add(session); err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrSessionStore, err)
	}

	return session, nil
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

func (s *SupervisorController) onClosed(_ api.ID, err error) {
	s.Close(err)
}

func (s *SupervisorController) SetCurrentSession(id api.ID) error {
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

func (s *SupervisorController) Detach() error {
	// Request detach to sesssion
	if err := sr.Detach(); err != nil {
		return fmt.Errorf("%w: %v", errdefs.ErrDetachSession, err)
	}

	// sr.Close(fmt.Errorf("session detached"))
	return nil
}
