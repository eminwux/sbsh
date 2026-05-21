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

package shared

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestWriteMetadata_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	type meta struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	in := meta{Name: "term-1", N: 7}

	if err := WriteMetadata(context.Background(), in, dir); err != nil {
		t.Fatalf("WriteMetadata error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	if raw[len(raw)-1] != '\n' {
		t.Errorf("metadata.json does not end with a trailing newline")
	}
	var out meta
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}

	// File mode should be 0o644.
	info, err := os.Stat(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("metadata.json mode = %o, want 0644", perm)
	}
}

func TestWriteMetadata_MarshalError(t *testing.T) {
	// A channel cannot be JSON-marshaled → marshal error path.
	if err := WriteMetadata(context.Background(), make(chan int), t.TempDir()); err == nil {
		t.Errorf("WriteMetadata(chan) = nil; want marshal error")
	}
}

func TestWriteMetadata_WriteErrorOnMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := WriteMetadata(context.Background(), map[string]int{"a": 1}, missing); err == nil {
		t.Errorf("WriteMetadata into missing dir = nil; want write error")
	}
}

func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "f.txt")
	if err := atomicWriteFile(dst, []byte("first"), 0o600); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := atomicWriteFile(dst, []byte("second"), 0o600); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("contents = %q, want %q", got, "second")
	}
}

func TestAtomicWriteFile_CreateTempError(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "no-such-dir", "f.txt")
	if err := atomicWriteFile(dst, []byte("x"), 0o600); err == nil {
		t.Errorf("atomicWriteFile into missing dir = nil; want CreateTemp error")
	}
}

// debugWriter captures slog text output so logging helpers can be exercised.
func newDebugLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLogBytes_EmptyAndNonEmpty(t *testing.T) {
	logger := newDebugLogger()
	// Empty data exercises the empty branches of logASCII and logHEX.
	LogBytes("p", nil, logger)
	// Non-empty multi-line data exercises the hex-dump loop and ASCII filter,
	// including non-printable bytes (rendered as '.') and a length > 16.
	data := make([]byte, 20)
	for i := range data {
		data[i] = byte(i)
	}
	data[5] = 'A' // a printable byte mixed in
	LogBytes("p", data, logger)
}

func TestLoggingConn_ReadWrite(t *testing.T) {
	logger := newDebugLogger()
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	lc := &LoggingConn{Conn: c1, Logger: logger, PrefixWrite: "w", PrefixRead: "r"}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 16)
		n, err := c2.Read(buf)
		if err != nil {
			t.Errorf("peer read: %v", err)
			return
		}
		if string(buf[:n]) != "hello" {
			t.Errorf("peer read = %q, want hello", buf[:n])
		}
		if _, err := c2.Write([]byte("ack")); err != nil {
			t.Errorf("peer write: %v", err)
		}
	}()

	if _, err := lc.Write([]byte("hello")); err != nil {
		t.Fatalf("LoggingConn.Write: %v", err)
	}
	buf := make([]byte, 16)
	n, err := lc.Read(buf)
	if err != nil {
		t.Fatalf("LoggingConn.Read: %v", err)
	}
	if string(buf[:n]) != "ack" {
		t.Errorf("LoggingConn.Read = %q, want ack", buf[:n])
	}
	wg.Wait()
}

// unixConnPair returns a connected pair of *net.UnixConn over a temp socket.
func unixConnPair(t *testing.T) (server, client *net.UnixConn) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()

	accepted := make(chan *net.UnixConn, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			accepted <- nil
			return
		}
		accepted <- c.(*net.UnixConn)
	}()

	dialed, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial unix: %v", err)
	}
	server = <-accepted
	if server == nil {
		t.Fatal("accept failed")
	}
	client = dialed.(*net.UnixConn)
	t.Cleanup(func() { server.Close(); client.Close() })
	return server, client
}

func TestLoggingConnUnix_ReadWrite(t *testing.T) {
	logger := newDebugLogger()
	server, client := unixConnPair(t)

	lc := &LoggingConnUnix{UnixConn: client, Logger: logger, PrefixWrite: "w", PrefixRead: "r"}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 16)
		n, err := server.Read(buf)
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if _, err := server.Write(buf[:n]); err != nil {
			t.Errorf("server echo: %v", err)
		}
	}()

	if _, err := lc.Write([]byte("ping")); err != nil {
		t.Fatalf("LoggingConnUnix.Write: %v", err)
	}
	buf := make([]byte, 16)
	n, err := lc.Read(buf)
	if err != nil {
		t.Fatalf("LoggingConnUnix.Read: %v", err)
	}
	if string(buf[:n]) != "ping" {
		t.Errorf("LoggingConnUnix.Read = %q, want ping", buf[:n])
	}
	wg.Wait()
}

func TestLoggingConnUnix_MsgRoundTrip(t *testing.T) {
	logger := newDebugLogger()
	server, client := unixConnPair(t)

	lc := &LoggingConnUnix{UnixConn: client, Logger: logger, PrefixWrite: "w", PrefixRead: "r"}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Reflect a message back (no OOB).
		if _, err := server.Write([]byte("msg")); err != nil {
			t.Errorf("server write: %v", err)
		}
	}()

	// Write a message with empty OOB (exercises the no-OOB branch).
	if _, _, err := lc.WriteMsgUnix([]byte("out"), nil, nil); err != nil {
		t.Fatalf("WriteMsgUnix: %v", err)
	}

	buf := make([]byte, 16)
	oob := make([]byte, 16)
	n, _, _, err := lc.ReadMsgUnix(buf, oob)
	if err != nil {
		t.Fatalf("ReadMsgUnix: %v", err)
	}
	if string(buf[:n]) != "msg" {
		t.Errorf("ReadMsgUnix payload = %q, want msg", buf[:n])
	}
	wg.Wait()
}

func TestLoggingConnUnix_MsgWithFDPassing(t *testing.T) {
	logger := newDebugLogger()
	server, client := unixConnPair(t)

	sender := &LoggingConnUnix{UnixConn: client, Logger: logger, PrefixWrite: "w", PrefixRead: "r"}
	receiver := &LoggingConnUnix{UnixConn: server, Logger: logger, PrefixWrite: "w", PrefixRead: "r"}

	// Open a file whose descriptor we send over the socket as OOB rights —
	// this exercises the control-message (FD) branches of both methods.
	f, err := os.CreateTemp(t.TempDir(), "fd-*.txt")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()

	rights := unix.UnixRights(int(f.Fd()))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, _, err := sender.WriteMsgUnix([]byte("fd"), rights, nil); err != nil {
			t.Errorf("WriteMsgUnix with rights: %v", err)
		}
	}()

	buf := make([]byte, 16)
	oob := make([]byte, 1024)
	n, oobn, _, err := receiver.ReadMsgUnix(buf, oob)
	if err != nil {
		t.Fatalf("ReadMsgUnix: %v", err)
	}
	if string(buf[:n]) != "fd" {
		t.Errorf("payload = %q, want fd", buf[:n])
	}
	if oobn == 0 {
		t.Fatalf("oobn = 0; expected control message bytes")
	}
	// Close any received descriptors to avoid leaks.
	cmsgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		t.Fatalf("ParseSocketControlMessage: %v", err)
	}
	for _, m := range cmsgs {
		if fds, perr := unix.ParseUnixRights(&m); perr == nil {
			for _, fd := range fds {
				unix.Close(fd)
			}
		}
	}
	wg.Wait()
}
