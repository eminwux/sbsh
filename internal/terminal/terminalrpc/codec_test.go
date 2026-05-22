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
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/rpc"
	"os"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

func newTestCodec(t *testing.T, srv *net.UnixConn) *unixJSONServerCodec {
	t.Helper()
	return NewUnixJSONServerCodec(srv, slog.New(slog.NewTextHandler(io.Discard, nil))).(*unixJSONServerCodec)
}

// readWireResponse reads one JSON object the codec wrote to the client side.
func readWireResponse(t *testing.T, cli *net.UnixConn) struct {
	ID     any             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  any             `json:"error"`
} {
	t.Helper()
	var resp struct {
		ID     any             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := json.NewDecoder(cli).Decode(&resp); err != nil {
		t.Fatalf("decode wire response: %v", err)
	}
	return resp
}

// TestReadRequestHeader_DecodeError covers the decode-failure branch: a
// truncated/garbage frame must surface the decoder error rather than route a
// bogus request.
func TestReadRequestHeader_DecodeError(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	mustWriteJSON(t, cli, `not-json`)
	_ = cli.Close()

	var r rpc.Request
	if err := codec.ReadRequestHeader(&r); err == nil {
		t.Fatal("ReadRequestHeader: want decode error, got nil")
	}
}

// TestWriteResponse_NormalResolvesWireID drives a request through the header
// reader so WriteResponse can resolve the original wire id, then asserts the
// response carries that id and the encoded result.
func TestWriteResponse_NormalResolvesWireID(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	mustWriteJSON(t, cli, `{"id":7,"method":"Svc.M","params":[{}]}`)

	var req rpc.Request
	if err := codec.ReadRequestHeader(&req); err != nil {
		t.Fatalf("ReadRequestHeader: %v", err)
	}

	body := struct {
		Message string `json:"Message"`
	}{Message: "ok"}
	resp := &rpc.Response{ServiceMethod: "Svc.M", Seq: req.Seq}
	if err := codec.WriteResponse(resp, &body); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	wire := readWireResponse(t, cli)
	if id, ok := wire.ID.(float64); !ok || id != 7 {
		t.Fatalf("wire id = %v; want 7", wire.ID)
	}
	if wire.Error != nil {
		t.Fatalf("wire error = %v; want nil", wire.Error)
	}
}

// TestWriteResponse_ErrorSetsErrorField asserts a non-empty resp.Error yields
// the jsonrpc error shape: null result, error string set.
func TestWriteResponse_ErrorSetsErrorField(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	mustWriteJSON(t, cli, `{"id":"abc","method":"Svc.M","params":[{}]}`)

	var req rpc.Request
	if err := codec.ReadRequestHeader(&req); err != nil {
		t.Fatalf("ReadRequestHeader: %v", err)
	}

	resp := &rpc.Response{ServiceMethod: "Svc.M", Seq: req.Seq, Error: "boom"}
	if err := codec.WriteResponse(resp, struct{}{}); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	wire := readWireResponse(t, cli)
	if wire.ID != "abc" {
		t.Fatalf("wire id = %v; want abc", wire.ID)
	}
	if string(wire.Result) != "null" {
		t.Fatalf("wire result = %s; want null", wire.Result)
	}
	if wire.Error != "boom" {
		t.Fatalf("wire error = %v; want boom", wire.Error)
	}
}

// TestWriteResponse_UnknownSeqFallsBackToServerSeq covers the !hasID branch:
// when no wire id was stashed for the seq, the server seq is used as the id.
func TestWriteResponse_UnknownSeqFallsBackToServerSeq(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	resp := &rpc.Response{ServiceMethod: "Svc.M", Seq: 42}
	if err := codec.WriteResponse(resp, struct{}{}); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	wire := readWireResponse(t, cli)
	if id, ok := wire.ID.(float64); !ok || id != 42 {
		t.Fatalf("wire id = %v; want 42 (server-seq fallback)", wire.ID)
	}
}

// TestWriteResponse_EncodeError covers the marshal-failure branch: an
// unencodable body (a channel) makes the JSON encoder fail and the error
// propagates out.
func TestWriteResponse_EncodeError(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	resp := &rpc.Response{ServiceMethod: "Svc.M", Seq: 1}
	if err := codec.WriteResponse(resp, make(chan int)); err == nil {
		t.Fatal("WriteResponse: want encode error for unencodable body, got nil")
	}
}

// TestWriteResponse_FDPassing covers the SCM_RIGHTS path: a *ResponseWithFD
// with a live fd is sent via WriteMsgUnix, the received message carries both
// the JSON payload and a dup'd fd, and the codec clears r.FDs so a retry can't
// double-close.
func TestWriteResponse_FDPassing(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	mustWriteJSON(t, cli, `{"id":3,"method":"Svc.Attach","params":[{}]}`)
	var req rpc.Request
	if err := codec.ReadRequestHeader(&req); err != nil {
		t.Fatalf("ReadRequestHeader: %v", err)
	}

	// A throwaway pipe gives us a real fd to pass.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()

	r := &api.ResponseWithFD{JSON: map[string]string{"k": "v"}, FDs: []int{int(pr.Fd())}}
	resp := &rpc.Response{ServiceMethod: "Svc.Attach", Seq: req.Seq}
	if err := codec.WriteResponse(resp, r); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	if r.FDs != nil {
		t.Fatalf("r.FDs = %v; want nil after send", r.FDs)
	}

	buf := make([]byte, 4096)
	oob := make([]byte, 256)
	n, oobn, _, _, err := cli.ReadMsgUnix(buf, oob)
	if err != nil {
		t.Fatalf("ReadMsgUnix: %v", err)
	}
	if oobn == 0 {
		t.Fatal("ReadMsgUnix: no OOB data; want passed fd")
	}
	scms, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		t.Fatalf("ParseSocketControlMessage: %v", err)
	}
	fds, err := unix.ParseUnixRights(&scms[0])
	if err != nil {
		t.Fatalf("ParseUnixRights: %v", err)
	}
	for _, fd := range fds {
		_ = unix.Close(fd)
	}
	if len(fds) != 1 {
		t.Fatalf("received %d fds; want 1", len(fds))
	}

	var wire struct {
		ID     any             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(buf[:n], &wire); err != nil {
		t.Fatalf("unmarshal fd-path payload: %v", err)
	}
	if id, ok := wire.ID.(float64); !ok || id != 3 {
		t.Fatalf("wire id = %v; want 3", wire.ID)
	}
	if string(wire.Result) != `{"k":"v"}` {
		t.Fatalf("wire result = %s; want {\"k\":\"v\"}", wire.Result)
	}
}

// TestWriteResponse_FDPassingEncodeError covers the encode-failure branch
// inside the FD path: an unencodable JSON payload makes the encoder fail, and
// the deferred fd-close still runs (no leak, no double-close).
func TestWriteResponse_FDPassingEncodeError(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()
	defer srv.Close()

	codec := newTestCodec(t, srv)
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()

	r := &api.ResponseWithFD{JSON: make(chan int), FDs: []int{int(pr.Fd())}}
	resp := &rpc.Response{ServiceMethod: "Svc.Attach", Seq: 1}
	if err := codec.WriteResponse(resp, r); err == nil {
		t.Fatal("WriteResponse: want encode error for unencodable FD payload, got nil")
	}
	if r.FDs != nil {
		t.Fatalf("r.FDs = %v; want nil even on encode error", r.FDs)
	}
}

func TestCodecClose(t *testing.T) {
	cli, srv := mustSocketpair(t)
	defer cli.Close()

	codec := newTestCodec(t, srv)
	if err := codec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

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
