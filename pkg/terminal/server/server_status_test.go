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

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/eminwux/sbsh/pkg/terminal/server"
)

// statusServer is a Server started on a temp layout with a caller-chosen
// workload and options. MetadataDir is always set so tests can read the
// document from disk.
type statusServer struct {
	srv         *server.Server
	socketPath  string
	metadataDir string
	serveErrCh  chan error
}

func startStatusServer(
	ctx context.Context,
	t *testing.T,
	args []string,
	mutate func(*api.TerminalSpec),
	opts ...server.Option,
) *statusServer {
	t.Helper()
	tmp := t.TempDir()
	socketPath := filepath.Join(tmp, "ctrl.sock")
	metadataDir := filepath.Join(tmp, "meta")
	if err := os.Mkdir(metadataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	spec := &api.TerminalSpec{
		ID:            api.ID("status-" + filepath.Base(tmp)),
		Name:          "status-test",
		Labels:        map[string]string{},
		Command:       "/bin/sh",
		CommandArgs:   args,
		EnvInherit:    true,
		RunPath:       tmp,
		MetadataDir:   metadataDir,
		SocketFile:    socketPath,
		CaptureFile:   filepath.Join(tmp, "capture.log"),
		ShutdownGrace: 500 * time.Millisecond,
	}
	if mutate != nil {
		mutate(spec)
	}
	srv, err := server.New(spec, slog.New(slog.NewTextHandler(io.Discard, nil)), opts...)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	s := &statusServer{srv: srv, socketPath: socketPath, metadataDir: metadataDir, serveErrCh: make(chan error, 1)}
	go func() { s.serveErrCh <- srv.Serve(ctx, listener) }()
	t.Cleanup(func() {
		_ = srv.Stop(errors.New("test cleanup"))
		select {
		case <-s.serveErrCh:
		case <-time.After(5 * time.Second):
		}
	})
	return s
}

func (s *statusServer) waitServe(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-s.serveErrCh:
		// Refill so the cleanup's drain does not wait out its timeout.
		s.serveErrCh <- err
		return err
	case <-time.After(timeout):
		t.Fatalf("Serve did not return within %v", timeout)
		return nil
	}
}

func TestServer_WorkloadExitCode3(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := startStatusServer(ctx, t, []string{"-c", "sleep 1; exit 3"}, nil)

	if err := waitReady(ctx, s.srv, 10*time.Second); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	md, err := s.srv.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if md.Status.ExitCode != nil || md.Status.ExitSignal != "" || !md.Status.ExitedAt.IsZero() {
		t.Fatalf("running terminal has exit details: %+v", md.Status)
	}

	serveErr := s.waitServe(t, 10*time.Second)
	var ee *server.ExitError
	if !errors.As(serveErr, &ee) {
		t.Fatalf("Serve err %v (%T) is not a *server.ExitError", serveErr, serveErr)
	}
	if ee.Code == nil || *ee.Code != 3 || ee.Signal != "" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want code 3", ee.Code, ee.Signal)
	}

	md, err = s.srv.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if md.Status.State != api.Exited || md.Status.ExitCode == nil || *md.Status.ExitCode != 3 ||
		md.Status.ExitedAt.IsZero() {
		t.Fatalf("final status = %+v, want Exited, exitCode 3, exitedAt", md.Status)
	}
	disk := readDoc(t, filepath.Join(s.metadataDir, "metadata.json"))
	if disk.Status.State != api.Exited || disk.Status.ExitCode == nil || *disk.Status.ExitCode != 3 {
		t.Fatalf("persisted status = %+v, want Exited with exitCode 3", disk.Status)
	}
}

func TestServer_WorkloadKilledBySIGKILL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := startStatusServer(ctx, t, []string{"-c", "sleep 1; kill -9 $$"}, nil)

	serveErr := s.waitServe(t, 10*time.Second)
	var ee *server.ExitError
	if !errors.As(serveErr, &ee) {
		t.Fatalf("Serve err %v (%T) is not a *server.ExitError", serveErr, serveErr)
	}
	if ee.Code != nil || ee.Signal != "SIGKILL" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want SIGKILL", ee.Code, ee.Signal)
	}
	disk := readDoc(t, filepath.Join(s.metadataDir, "metadata.json"))
	if disk.Status.ExitSignal != "SIGKILL" || disk.Status.ExitCode != nil || disk.Status.ExitedAt.IsZero() {
		t.Fatalf("persisted status = %+v, want exitSignal SIGKILL, no exitCode", disk.Status)
	}
}

// TestServer_StopIsNotExitError asserts a non-workload cause is not an
// ExitError, while the exit details of the child Stop killed are still
// persisted by the second close-time write.
func TestServer_StopIsNotExitError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := startStatusServer(ctx, t, []string{"-c", "sleep 30"}, nil)
	if err := waitReady(ctx, s.srv, 10*time.Second); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	_ = s.srv.Stop(errors.New("operator stop"))
	serveErr := s.waitServe(t, 10*time.Second)
	var ee *server.ExitError
	if errors.As(serveErr, &ee) {
		t.Fatalf("Stop cause %v matched *server.ExitError", serveErr)
	}
	disk := readDoc(t, filepath.Join(s.metadataDir, "metadata.json"))
	if disk.Status.State != api.Exited || disk.Status.ExitedAt.IsZero() ||
		(disk.Status.ExitCode == nil && disk.Status.ExitSignal == "") {
		t.Fatalf("persisted status after Stop = %+v, want Exited with exit details", disk.Status)
	}
}

func TestServer_MetadataFileCustomName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := startStatusServer(ctx, t, []string{"-c", "sleep 1; exit 3"}, func(spec *api.TerminalSpec) {
		spec.MetadataFile = "terminal.json"
	})
	_ = s.waitServe(t, 10*time.Second)

	doc := readDoc(t, filepath.Join(s.metadataDir, "terminal.json"))
	if doc.Status.State != api.Exited || doc.Spec.MetadataFile != "terminal.json" {
		t.Fatalf("terminal.json status = %+v, want Exited", doc.Status)
	}
	if _, err := os.Stat(filepath.Join(s.metadataDir, "metadata.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("metadata.json exists next to terminal.json (stat err %v)", err)
	}
}

func TestServer_New_RejectsBadMetadataFileAndNilObserver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := map[string]struct {
		spec api.TerminalSpec
		opts []server.Option
	}{
		"separator":          {spec: api.TerminalSpec{Command: "/bin/sh", MetadataDir: "/tmp/x", MetadataFile: "a/b"}},
		"custom without dir": {spec: api.TerminalSpec{Command: "/bin/sh", MetadataFile: "terminal.json"}},
		"nil observer": {
			spec: api.TerminalSpec{Command: "/bin/sh"},
			opts: []server.Option{server.WithStatusObserver(nil)},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := server.New(&tc.spec, logger, tc.opts...); err == nil {
				t.Fatal("New: want error")
			}
		})
	}
}

// observed records observer calls for server-level tests.
type observed struct {
	mu   sync.Mutex
	docs []api.TerminalDoc
}

func (o *observed) fn(doc api.TerminalDoc) {
	o.mu.Lock()
	o.docs = append(o.docs, doc)
	o.mu.Unlock()
}

func (o *observed) snapshot() []api.TerminalDoc {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]api.TerminalDoc(nil), o.docs...)
}

func (o *observed) waitFor(t *testing.T, timeout time.Duration, pred func([]api.TerminalDoc) bool) []api.TerminalDoc {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if docs := o.snapshot(); pred(docs) {
			return docs
		}
		time.Sleep(20 * time.Millisecond)
	}
	docs := o.snapshot()
	t.Fatalf("observer condition not met within %v; got %d docs", timeout, len(docs))
	return docs
}

// TestServer_StatusObserver_LifecycleInOrder: a terminal that starts and
// exits produces an in-order state sequence that reaches Ready and ends in
// Exited with the exit code.
func TestServer_StatusObserver_LifecycleInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	o := &observed{}
	s := startStatusServer(ctx, t, []string{"-c", "sleep 1; exit 3"}, nil, server.WithStatusObserver(o.fn))
	_ = s.waitServe(t, 10*time.Second)

	// The final call may land after Serve returns.
	docs := o.waitFor(t, 5*time.Second, func(d []api.TerminalDoc) bool {
		return len(d) > 0 && d[len(d)-1].Status.State == api.Exited && d[len(d)-1].Status.ExitCode != nil
	})
	sawReady := false
	for i, d := range docs {
		if d.Status.State == api.Ready {
			sawReady = true
		}
		if i > 0 && d.Status.State < docs[i-1].Status.State {
			t.Fatalf("state went backwards at %d: %v after %v", i, d.Status.State, docs[i-1].Status.State)
		}
	}
	if !sawReady {
		t.Fatal("observer never saw Ready")
	}
	if last := docs[len(docs)-1]; *last.Status.ExitCode != 3 || last.Status.ExitedAt.IsZero() {
		t.Fatalf("final observed status = %+v, want exitCode 3 and exitedAt", last.Status)
	}
}

// TestServer_StatusObserver_AttachDetach: one call carrying the attacher
// on attach, and one without it on detach.
func TestServer_StatusObserver_AttachDetach(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	o := &observed{}
	s := startStatusServer(ctx, t, []string{}, nil, server.WithStatusObserver(o.fn))
	if err := waitReady(ctx, s.srv, 10*time.Second); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	client := rpcclient.NewUnix(s.socketPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer client.Close()

	id := api.ID("observer-attacher")
	conn, err := client.Attach(ctx, &api.AttachRequest{ClientID: id}, nil)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer conn.Close()
	hasAttacher := func(d api.TerminalDoc) bool {
		for _, a := range d.Status.Attachers {
			if a == string(id) {
				return true
			}
		}
		return false
	}
	o.waitFor(t, 5*time.Second, func(d []api.TerminalDoc) bool {
		return len(d) > 0 && hasAttacher(d[len(d)-1])
	})

	if derr := client.Detach(ctx, &id); derr != nil {
		t.Fatalf("Detach: %v", derr)
	}
	o.waitFor(t, 5*time.Second, func(d []api.TerminalDoc) bool {
		return len(d) > 0 && !hasAttacher(d[len(d)-1])
	})
}

// TestServer_StatusObserver_SleepingObserverDoesNotDelay: an observer that
// sleeps 5s per call does not slow PTY round trips or shutdown.
func TestServer_StatusObserver_SleepingObserverDoesNotDelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := startStatusServer(ctx, t, []string{}, nil, server.WithStatusObserver(func(api.TerminalDoc) {
		time.Sleep(5 * time.Second)
	}))
	if err := waitReady(ctx, s.srv, 4*time.Second); err != nil {
		t.Fatalf("waitReady with sleeping observer: %v", err)
	}
	client := rpcclient.NewUnix(s.socketPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer client.Close()

	subConn, err := client.Subscribe(ctx, &api.SubscribeRequest{ClientID: "sleepy-sub"}, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subConn.Close()
	runWrite(ctx, t, client)
	runSubscribeRead(t, subConn)

	start := time.Now()
	_ = s.srv.Stop(errors.New("stop with sleeping observer"))
	_ = s.waitServe(t, 4*time.Second)
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("Serve took %v to return after Stop with a sleeping observer", d)
	}
}

func readDoc(t *testing.T, path string) api.TerminalDoc {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc api.TerminalDoc
	if uerr := json.Unmarshal(b, &doc); uerr != nil {
		t.Fatalf("unmarshal %s: %v", path, uerr)
	}
	return doc
}
