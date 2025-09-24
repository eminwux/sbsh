package supervisor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
	"sbsh/pkg/supervisor/supervisorrunner"
	"testing"
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

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
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
			IDFunc: func() api.SessionID {
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

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, ErrOpenSocketCtrl) {
		t.Fatalf("expected '%v'; got: '%v'", ErrOpenSocketCtrl, err)
	}

}
func Test_ErrStartRPCServer(t *testing.T) {
	// ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
			Ctx: ctx,
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
			IDFunc: func() api.SessionID {
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

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, ErrStartRPCServer) {
		t.Fatalf("expected '%v'; got: '%v'", ErrStartRPCServer, err)
	}

}

func Test_ErrStartSession(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
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
				return fmt.Errorf("force session start fail")
			},
			IDFunc: func() api.SessionID {
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

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, ErrStartSession) {
		t.Fatalf("expected '%v'; got: '%v'", ErrStartSession, err)
	}

}

func Test_ErrContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(ctx)

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
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
			IDFunc: func() api.SessionID {
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

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	<-ctrlReady
	cancel()

	if err := <-exitCh; err != nil && !errors.Is(err, ErrContextDone) {
		t.Fatalf("expected '%v'; got: '%v'", ErrContextDone, err)
	}

}

func Test_ErrRPCServerExited(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
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
			IDFunc: func() api.SessionID {
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

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	<-ctrlReady
	rpcDoneCh <- fmt.Errorf("force rpc server exit")

	if err := <-exitCh; err != nil && !errors.Is(err, ErrRPCServerExited) {
		t.Fatalf("expected '%v'; got: '%v'", ErrRPCServerExited, err)
	}

}

func Test_ErrSessionExists(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
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
			IDFunc: func() api.SessionID {
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

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	<-ctrlReady
	closeReqCh <- fmt.Errorf("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", ErrCloseReq, err)
	}

}

func Test_ErrCloseReq(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
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
			IDFunc: func() api.SessionID {
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

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	<-ctrlReady
	closeReqCh <- fmt.Errorf("force close request")

	if err := <-exitCh; err != nil && !errors.Is(err, ErrCloseReq) {
		t.Fatalf("expected '%v'; got: '%v'", ErrCloseReq, err)
	}

}

func Test_ErrSessionStore(t *testing.T) {
	sessionCtrl := NewSupervisorController(context.Background())

	newSupervisorRunner = func(ctx context.Context) supervisorrunner.SupervisorRunner {
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
			IDFunc: func() api.SessionID {
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
			GetFunc: func(id api.SessionID) (*sessionstore.SupervisedSession, bool) {
				return nil, false
			},
			ListLiveFunc: func() []api.SessionID {
				return []api.SessionID{}
			},
			RemoveFunc: func(id api.SessionID) {
			},
			CurrentFunc: func() api.SessionID {
				return "sess-1"
			},
			SetCurrentFunc: func(id api.SessionID) error {
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

	exitCh := make(chan error)

	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run()
	}(exitCh)

	if err := <-exitCh; err != nil && !errors.Is(err, ErrSessionStore) {
		t.Fatalf("expected '%v'; got: '%v'", ErrSessionStore, err)
	}

}
