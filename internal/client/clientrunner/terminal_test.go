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

package clientrunner

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/creack/pty"
)

// openTTY opens a pty pair and returns the slave usable as a real terminal
// handle. Both sides are closed on cleanup.
func openTTY(t *testing.T) *os.File {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open unavailable in this environment: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = ptmx.Close()
	})
	return tty
}

// unixSocketPair returns a connected pair of *net.UnixConn over a temp socket.
func unixSocketPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "io.sock")
	ln, errListen := net.Listen("unix", sockPath)
	if errListen != nil {
		t.Fatalf("listen: %v", errListen)
	}
	t.Cleanup(func() { _ = ln.Close() })

	type acc struct {
		c   net.Conn
		err error
	}
	accCh := make(chan acc, 1)
	go func() {
		c, errAccept := ln.Accept()
		accCh <- acc{c, errAccept}
	}()

	dialed, errDial := net.Dial("unix", sockPath)
	if errDial != nil {
		t.Fatalf("dial: %v", errDial)
	}
	got := <-accCh
	if got.err != nil {
		t.Fatalf("accept: %v", got.err)
	}

	client := dialed.(*net.UnixConn)
	server := got.c.(*net.UnixConn)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, server
}

func newTerminalExec(t *testing.T) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		metadataMu: sync.RWMutex{},
	}
}

// TestToRawModeAndRestore drives toBashUIMode (raw) followed by toExitShell
// (restore) against a real pty slave, covering toRawMode and the restore path.
func TestToRawModeAndRestore(t *testing.T) {
	tty := openTTY(t)
	sr := newTerminalExec(t)
	sr.stdin = tty

	if err := sr.toBashUIMode(); err != nil {
		t.Fatalf("toBashUIMode: %v", err)
	}
	if sr.uiMode != UIBash {
		t.Errorf("uiMode = %v; want UIBash", sr.uiMode)
	}
	if sr.lastTermState == nil {
		t.Fatal("toBashUIMode did not capture lastTermState")
	}

	if err := sr.toExitShell(); err != nil {
		t.Fatalf("toExitShell: %v", err)
	}
	if sr.uiMode != UIExitShell {
		t.Errorf("uiMode = %v; want UIExitShell", sr.uiMode)
	}
}

// TestToExitShell_NilState verifies toExitShell is a no-op (no panic, no error)
// when no raw state was ever captured.
func TestToExitShell_NilState(t *testing.T) {
	sr := newTerminalExec(t)
	if err := sr.toExitShell(); err != nil {
		t.Fatalf("toExitShell with nil state: %v", err)
	}
	if sr.uiMode != UIExitShell {
		t.Errorf("uiMode = %v; want UIExitShell", sr.uiMode)
	}
}

// TestToRawMode_NonTTYFails verifies toRawMode surfaces an error for a
// non-terminal handle (a pipe).
func TestToRawMode_NonTTYFails(t *testing.T) {
	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	if _, errRaw := toRawMode(testLogger(), r); errRaw == nil {
		t.Fatal("toRawMode on a pipe returned nil error")
	}
}

// TestToBashUIMode_NonTTYError covers the error branch where raw mode cannot
// be set because stdin is not a terminal.
func TestToBashUIMode_NonTTYError(t *testing.T) {
	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	sr := newTerminalExec(t)
	sr.stdin = r
	if errMode := sr.toBashUIMode(); errMode == nil {
		t.Fatal("toBashUIMode on a non-tty returned nil error")
	}
}

// TestWriteTerminal_Error covers the retry-then-fail branch when the
// underlying connection is closed before writing.
func TestWriteTerminal_Error(t *testing.T) {
	client, server := unixSocketPair(t)
	_ = server.Close()
	_ = client.Close() // force every write to fail

	sr := newTerminalExec(t)
	sr.ioConn = client
	if err := sr.writeTerminal("x"); err == nil {
		t.Fatal("writeTerminal returned nil error writing to a closed conn")
	}
}

// TestInitTerminal is a trivial smoke of the no-op initialiser.
func TestInitTerminal(t *testing.T) {
	sr := newTerminalExec(t)
	if err := sr.initTerminal(); err != nil {
		t.Fatalf("initTerminal: %v", err)
	}
}

// TestWriteTerminal writes a string through ioConn and confirms the peer
// receives every byte.
func TestWriteTerminal(t *testing.T) {
	client, server := unixSocketPair(t)
	sr := newTerminalExec(t)
	sr.ioConn = client

	const payload = "hello"
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(payload))
		n, _ := readFull(server, buf)
		readDone <- buf[:n]
	}()

	if err := sr.writeTerminal(payload); err != nil {
		t.Fatalf("writeTerminal: %v", err)
	}
	got := <-readDone
	if string(got) != payload {
		t.Fatalf("peer received %q; want %q", got, payload)
	}
}

// readFull reads len(buf) bytes or until error.
func readFull(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
