package supervisor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
	"sbsh/pkg/supervisor/supervisorrunner"
	"testing"

	"github.com/spf13/viper"
)

type fakeListener struct{}

func (f *fakeListener) Accept() (net.Conn, error) {
	// No real conns; make it obvious if code tries to use it
	return nil, errors.New("stub listener: Accept not implemented")
}
func (f *fakeListener) Close() error { return nil }
func (f *fakeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// use in test
func newStubListener() net.Listener { return &fakeListener{} }

func Test_ErrOpenSocketCtrl(t *testing.T) {
	// ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, fmt.Errorf("force socket fail")
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSocketCtrl, err)
	}

}
func Test_ErrStartRPCServer(t *testing.T) {
	// ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- fmt.Errorf("force server fail"):
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}
	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartRPCServer, err)
	}

}

func Test_ErrStartSession(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			StartSupervisorFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				return fmt.Errorf("force session start fail")
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrStartSession) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStartSession, err)
	}

}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(ctx)

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			StartSupervisorFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{}, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
	}

}

func Test_ErrRPCServerExited(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			StartSupervisorFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{}, 1)

	exitCh := make(chan error)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	rpcDoneCh <- fmt.Errorf("force rpc server exit")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrRPCServerExited, err)
	}

}

func Test_ErrSessionExists(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			StartSupervisorFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{}, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	closeReqCh <- fmt.Errorf("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}

}

func Test_ErrCloseReq(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			StartSupervisorFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{}, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	<-ctrlReady
	closeReqCh <- fmt.Errorf("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrCloseReq, err)
	}

}

func Test_ErrSessionStore(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(spec *api.SupervisorSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: spec.Ctx,
			OpenSocketCtrlFunc: func() (net.Listener, error) {
				// default: return nil listener and nil error
				return nil, nil
			},
			StartServerFunc: func(ctx context.Context, ln net.Listener, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, errCh chan error) {
				// default: immediately signal ready
				select {
				case readyCh <- nil:
				default:
				}
			},
			StartSessionFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				// default: do nothing and succeed
				return nil
			},
			StartSupervisorFunc: func(ctx context.Context, evCh chan<- supervisorrunner.SupervisorRunnerEvent) error {
				return nil
			},
			IDFunc: func() api.ID {
				// default: empty ID
				return ""
			},
			CloseFunc: func(reason error) error {
				// default: succeed
				return nil
			},
			ResizeFunc: func(args api.ResizeArgs) {
				// default: no-op
			},
		}
	}
	newSessionStore = func() sessionstore.SessionStore {
		return &sessionstore.SessionStoreTest{
			AddFunc: func(s *sessionstore.SupervisedSession) error {

				return fmt.Errorf("force add fail")
			},
			GetFunc: func(id api.ID) (*sessionstore.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.ID {
				return []api.ID{}
			},
			RemoveFunc: func(id api.ID) {
			},
			CurrentFunc: func() api.ID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.ID) error {
				if id == "" {
					return errors.New("empty id not allowed")
				}
				return nil
			},
		}

	}

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	closeReqCh = make(chan error, 1)
	rpcReadyCh = make(chan error)
	rpcDoneCh = make(chan error)
	eventsCh = make(chan supervisorrunner.SupervisorRunnerEvent, 32)
	ctrlReady = make(chan struct{}, 1)

	supervisorID := common.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		ID:      api.ID(supervisorID),
		Label:   "default",
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(&spec)
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrSessionStore) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSessionStore, err)
	}

}
