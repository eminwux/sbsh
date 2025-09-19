package supervisor

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"sbsh/pkg/api"
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

func Test_ErrSpecCmdMissing(t *testing.T) {
	// ctx, cancel := context.WithCancel(context.Background())
	sessionCtrl := NewSupervisorController(context.Background())

	// Define a new Session
	// spec := api.SessionSpec{
	// 	ID:          api.SessionID("abcdef"),
	// 	Kind:        api.SessLocal,
	// 	Label:       "default",
	// 	Command:     "",
	// 	CommandArgs: nil,
	// 	Env:         os.Environ(),
	// 	LogDir:      "/tmp/sbsh-logs/s0",
	// }

	newSupervisorRunner = func(spec *api.SessionSpec) supervisorrunner.SupervisorRunner {
		return &supervisorrunner.SupervisorRunnerTest{
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

	exitCh := make(chan error)
	go func(exitCh chan error) {
		exitCh <- sessionCtrl.Run(context.Background())
	}(exitCh)

	// if err := <-exitCh; err != nil && !errors.Is(err, ErrSpecCmdMissing) {
	// 	t.Fatalf("expected '%v'; got: '%v'", ErrSpecCmdMissing, err)
	// }

}
