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

package terminalrpc

import (
	"io"
	"log/slog"
	"net"
	"net/rpc"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// TestReadRequestBody_NilDoesNotCrossTalk drives the codec through the same
// header/body sequence net/rpc uses when it sees an unknown method: read the
// header, then ReadRequestBody(nil) to discard the body. If the codec leaves
// the prior request's params in paramsBySeq, the next ReadRequestBody picks
// the smallest-seq entry — the stale one — and the wrong handler gets the
// wrong params. Reproduction asserts the second request's body matches its
// own wire params.
func TestReadRequestBody_NilDoesNotCrossTalk(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := NewUnixJSONServerCodec(srv, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Request 1: unknown method, distinctive params. net/rpc will discard via
	// ReadRequestBody(nil) on this one.
	mustWriteJSON(t, cli, `{"id":1,"method":"Unknown.Method","params":[{"x":111}]}`)
	// Request 2: known shape, different distinctive params. Whatever the
	// handler receives should be {"x":222}, never {"x":111}.
	mustWriteJSON(t, cli, `{"id":2,"method":"Known.Method","params":[{"x":222}]}`)

	var r1 rpc.Request
	if err := codec.ReadRequestHeader(&r1); err != nil {
		t.Fatalf("ReadRequestHeader #1: %v", err)
	}
	if err := codec.ReadRequestBody(nil); err != nil {
		t.Fatalf("ReadRequestBody(nil) #1: %v", err)
	}

	var r2 rpc.Request
	if err := codec.ReadRequestHeader(&r2); err != nil {
		t.Fatalf("ReadRequestHeader #2: %v", err)
	}
	var body struct {
		X int `json:"x"`
	}
	if err := codec.ReadRequestBody(&body); err != nil {
		t.Fatalf("ReadRequestBody #2: %v", err)
	}
	if body.X != 222 {
		t.Fatalf("body.X = %d; want 222 (got cross-talk from request 1's params)", body.X)
	}
}

// TestReadRequestBody_NilDrainsPendingParams asserts paramsBySeq is empty
// after a ReadRequestBody(nil): the buffer must not survive the connection's
// lifetime for every unknown-method call.
func TestReadRequestBody_NilDrainsPendingParams(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := NewUnixJSONServerCodec(srv, slog.New(slog.NewTextHandler(io.Discard, nil))).(*unixJSONServerCodec)

	mustWriteJSON(t, cli, `{"id":1,"method":"Unknown.Method","params":[{"x":1}]}`)

	var r rpc.Request
	if err := codec.ReadRequestHeader(&r); err != nil {
		t.Fatalf("ReadRequestHeader: %v", err)
	}
	if err := codec.ReadRequestBody(nil); err != nil {
		t.Fatalf("ReadRequestBody(nil): %v", err)
	}

	codec.seqMu.Lock()
	n := len(codec.paramsBySeq)
	codec.seqMu.Unlock()
	if n != 0 {
		t.Fatalf("paramsBySeq has %d leaked entries after ReadRequestBody(nil); want 0", n)
	}
}

func mustSocketpair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	return fdToUnixConn(t, fds[0], "a"), fdToUnixConn(t, fds[1], "b")
}

func fdToUnixConn(t *testing.T, fd int, name string) *net.UnixConn {
	t.Helper()
	f := os.NewFile(uintptr(fd), name)
	c, err := net.FileConn(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("FileConn: %v", err)
	}
	uc, ok := c.(*net.UnixConn)
	if !ok {
		_ = c.Close()
		t.Fatalf("FileConn did not return *net.UnixConn (got %T)", c)
	}
	return uc
}

func mustWriteJSON(t *testing.T, c *net.UnixConn, payload string) {
	t.Helper()
	if _, err := c.Write([]byte(payload)); err != nil {
		t.Fatalf("write %q: %v", payload, err)
	}
}
