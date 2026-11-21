// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package attach_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/rpc"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	internalterminal "github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/attach"
	"github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"golang.org/x/sys/unix"
)

// socketpairConn returns both ends of a fresh AF_UNIX socketpair as
// *net.UnixConn. It stands in for a broker that connects to the terminal on
// the caller's behalf and passes the connected socket back: the client end
// is built with net.FileConn from a raw descriptor, as a received
// SCM_RIGHTS fd would be, and no filesystem path is involved.
func socketpairConn(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	toConn := func(fd int, name string) *net.UnixConn {
		f := os.NewFile(uintptr(fd), name)
		defer f.Close()
		c, errC := net.FileConn(f)
		if errC != nil {
			t.Fatalf("FileConn(%s): %v", name, errC)
		}
		uc, ok := c.(*net.UnixConn)
		if !ok {
			t.Fatalf("FileConn(%s) = %T; want *net.UnixConn", name, c)
		}
		return uc
	}
	return toConn(fds[0], "client"), toConn(fds[1], "server")
}

// dialFakeTerminal is a terminal control server reached only through
// pre-connected sockets: each dial() creates a socketpair, serves one end
// with the production FD-aware codec, and returns the other. It counts the
// control RPCs it receives and the dials that carried them.
type dialFakeTerminal struct {
	pings, states, attaches, resizes, detaches, dials atomic.Int32

	ioMu     sync.Mutex
	ioServer int

	srv    *rpc.Server
	logger *slog.Logger
}

func newDialFakeTerminal(t *testing.T, logger *slog.Logger) *dialFakeTerminal {
	t.Helper()
	f := &dialFakeTerminal{ioServer: -1, srv: rpc.NewServer(), logger: logger}
	t.Cleanup(f.closeIOServer)
	core := &internalterminal.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) {
			f.pings.Add(1)
			return &api.PingMessage{Message: "PONG"}, nil
		},
		StateFunc: func() (*api.TerminalStatusMode, error) {
			f.states.Add(1)
			s := api.Ready
			return &s, nil
		},
		AttachFunc: f.attach,
		ResizeFunc: func() { f.resizes.Add(1) },
		DetachFunc: func(_ *api.ID) error {
			f.detaches.Add(1)
			// Like the real terminal, drop the IO connection on detach so
			// the client's copy loop ends.
			f.closeIOServer()
			return nil
		},
	}
	if err := f.srv.RegisterName(api.TerminalService, &terminalrpc.TerminalControllerRPC{Core: core}); err != nil {
		t.Fatalf("RegisterName: %v", err)
	}
	return f
}

func (f *dialFakeTerminal) attach(_ *api.AttachRequest, resp *api.ResponseWithFD) error {
	f.attaches.Add(1)
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return err
	}
	f.ioMu.Lock()
	f.ioServer = fds[0]
	f.ioMu.Unlock()
	resp.FDs = []int{fds[1]}
	return nil
}

func (f *dialFakeTerminal) closeIOServer() {
	f.ioMu.Lock()
	defer f.ioMu.Unlock()
	if f.ioServer >= 0 {
		_ = unix.Close(f.ioServer)
		f.ioServer = -1
	}
}

func (f *dialFakeTerminal) dialer(t *testing.T) terminal.Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f.dials.Add(1)
		clientEnd, serverEnd := socketpairConn(t)
		go f.srv.ServeCodec(terminalrpc.NewUnixJSONServerCodec(serverEnd, f.logger))
		return clientEnd, nil
	}
}

func (f *dialFakeTerminal) calls() int32 {
	return f.pings.Load() + f.states.Load() + f.attaches.Load() + f.resizes.Load() + f.detaches.Load()
}

func (f *dialFakeTerminal) String() string {
	return fmt.Sprintf("pings=%d states=%d attaches=%d resizes=%d detaches=%d dials=%d",
		f.pings.Load(), f.states.Load(), f.attaches.Load(), f.resizes.Load(), f.detaches.Load(), f.dials.Load())
}

// waitForRun polls cond until it holds, failing if Run returns first or the
// deadline passes.
func waitForRun(t *testing.T, runDone <-chan error, f *dialFakeTerminal, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		select {
		case runErr := <-runDone:
			t.Fatalf("Run returned early while waiting for %s: %v", what, runErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s (%s)", what, f)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// openTestPTY returns a pty pair sized 24x80 whose output is drained.
func openTestPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})
	if errSz := pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}); errSz != nil {
		t.Fatalf("Setsize: %v", errSz)
	}
	// Drain the tty output so the client's writes never block.
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()
	return ptmx, tty
}

// TestRun_DialerAttachResizeDetach drives pkg/attach.Run with only
// Options.Dial (no SocketPath). Every dial hands out a pre-connected socket
// whose peer is served by the production FD-aware terminal RPC codec. The
// test covers the whole session: Ping, State, Attach (IO fd over
// SCM_RIGHTS), the initial Resize, a SIGWINCH-driven Resize, and a ^]^]
// detach that issues the Detach RPC — all through the dialer.
func TestRun_DialerAttachResizeDetach(t *testing.T) {
	// Not parallel: the test sends SIGWINCH to its own process.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fake := newDialFakeTerminal(t, logger)
	ptmx, tty := openTestPTY(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- attach.Run(ctx, attach.Options{
			Dial:   fake.dialer(t),
			Stdin:  tty,
			Stdout: tty,
			Stderr: tty,
			Logger: logger,
		})
	}()

	waitForRun(t, runDone, fake, "attach and initial resize", func() bool {
		return fake.attaches.Load() == 1 && fake.resizes.Load() >= 1
	})

	// SIGWINCH-driven resize. Re-send until observed: the forwarder installs
	// its signal handler right after the initial resize.
	before := fake.resizes.Load()
	if errSz := pty.Setsize(ptmx, &pty.Winsize{Rows: 40, Cols: 120}); errSz != nil {
		t.Fatalf("Setsize: %v", errSz)
	}
	waitForRun(t, runDone, fake, "SIGWINCH resize", func() bool {
		_ = syscall.Kill(os.Getpid(), syscall.SIGWINCH)
		time.Sleep(20 * time.Millisecond)
		return fake.resizes.Load() > before
	})

	// ^]^] detach keystroke on the user's terminal.
	if _, errW := ptmx.Write([]byte{0x1d, 0x1d}); errW != nil {
		t.Fatalf("write detach keystroke: %v", errW)
	}

	select {
	case runErr := <-runDone:
		if !errors.Is(runErr, attach.ErrDetached) {
			t.Fatalf("Run err = %v; want ErrDetached", runErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the detach keystroke")
	}

	if fake.pings.Load() < 1 || fake.states.Load() < 1 || fake.detaches.Load() != 1 {
		t.Fatalf("%s; want ping, state and one detach through the dialer", fake)
	}
	// Every server-side call arrived over a connection the dialer handed
	// out, one connection per call.
	if got, want := fake.dials.Load(), fake.calls(); got != want {
		t.Fatalf("dials = %d; want %d (one per control RPC; %s)", got, want, fake)
	}
}

// TestRun_DialAndSocketPathMutuallyExclusive verifies Run rejects options
// that set both SocketPath and Dial with a distinct sentinel.
func TestRun_DialAndSocketPathMutuallyExclusive(t *testing.T) {
	t.Parallel()
	err := attach.Run(context.Background(), attach.Options{
		SocketPath: "/nonexistent/terminal.sock",
		Dial:       func(context.Context) (net.Conn, error) { return nil, errors.New("unused") },
	})
	if !errors.Is(err, attach.ErrSocketPathAndDial) {
		t.Fatalf("Run err = %v; want ErrSocketPathAndDial", err)
	}
	if errors.Is(err, attach.ErrSocketPathRequired) {
		t.Fatalf("Run err = %v; must be distinct from ErrSocketPathRequired", err)
	}
}

// TestRun_DialNonUnixConnFailsClearly verifies a dialer returning a
// non-Unix connection makes Run fail with terminal.ErrNotUnixConn rather
// than a generic cast error.
func TestRun_DialNonUnixConnFailsClearly(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runErr := attach.Run(ctx, attach.Options{
		Dial: func(context.Context) (net.Conn, error) {
			a, b := net.Pipe()
			_ = b.Close()
			return a, nil
		},
		Stdin:                  r,
		Stdout:                 w,
		Stderr:                 w,
		DisableDetachKeystroke: true,
		Logger:                 discardLogger(),
	})
	if !errors.Is(runErr, terminal.ErrNotUnixConn) {
		t.Fatalf("Run err = %v; want terminal.ErrNotUnixConn", runErr)
	}
}
