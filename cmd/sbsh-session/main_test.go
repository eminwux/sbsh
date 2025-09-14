package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/session"
	"syscall"
	"testing"
	"time"
)

func TestRunSession_ContextCancelPath(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func() {},
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() {},
			StatusFunc:    func() string { return "" },
		}
	}
	t.Cleanup(func() { newSessionController = orig })

	done := make(chan struct{})
	go func() {
		runSession("s-ctx", "/bin/true", nil) // will block until ctx.Done()
		close(done)
	}()

	// Give Run() time to set ready, then signal the process (NotifyContext listens to SIGTERM/INT)
	time.Sleep(20 * time.Millisecond)
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runSession to return after SIGTERM")
	}

}

func TestRunSession_WaitReadyError(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func() {},
			WaitReadyFunc: func() error { return fmt.Errorf("not ready") },
			WaitCloseFunc: func() {},
		}
	}
	t.Cleanup(func() { newSessionController = orig })

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	runSession("s-wre", "/bin/true", nil)

	if !bytes.Contains(buf.Bytes(), []byte("controller not ready")) {
		t.Fatalf("expected 'controller not ready' in logs; got: %s", buf.String())
	}
}

func TestRunSession_NormalShutdown(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func() {},
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() {},
		}
	}

	t.Cleanup(func() { newSessionController = orig })

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	go runSession("s-wre", "/bin/true", nil)
	time.Sleep(20 * time.Millisecond)
	exit <- nil
	time.Sleep(20 * time.Millisecond)

	if !bytes.Contains(buf.Bytes(), []byte("normal shutdown")) {
		t.Fatalf("expected 'normal shutdown' in logs; got: %s", buf.String())
	}
}

func TestRunSession_ErrorShutdown(t *testing.T) {
	orig := newSessionController
	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
		return &session.FakeSessionController{
			Exit:          make(chan error),
			RunFunc:       func() {},
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() {},
		}
	}

	t.Cleanup(func() { newSessionController = orig })

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	go runSession("s-wre", "/bin/true", nil)
	time.Sleep(20 * time.Millisecond)
	exit <- fmt.Errorf("error in controller")
	time.Sleep(20 * time.Millisecond)

	if !bytes.Contains(buf.Bytes(), []byte("controller stopped with error")) {
		t.Fatalf("expected 'controller stopped with error' in logs; got: %s", buf.String())
	}
}

// func TestRunSession_WaitReadyError(t *testing.T) {
// 	orig := newSessionController
// 	newSessionController = session.NewFakeSessionController
// 	t.Cleanup(func() { newSessionController = orig })

// 	var buf bytes.Buffer
// 	old := log.Writer()
// 	log.SetOutput(&buf)
// 	t.Cleanup(func() { log.SetOutput(old) })

// 	runSession("s-wre", "/bin/true", nil)

// 	if !bytes.Contains(buf.Bytes(), []byte("controller not ready")) {
// 		t.Fatalf("expected 'controller not ready' in logs; got: %s", buf.String())
// 	}
// }

// func TestRunSession_ContextCancelPath(t *testing.T) {
// 	orig := newSessionController
// 	var fc *fakeCtrlCtxPath
// 	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
// 		fc = newFakeCtrlCtxPath(ctx, exit).(*fakeCtrlCtxPath)
// 		return fc
// 	}
// 	t.Cleanup(func() { newSessionController = orig })

// 	var buf bytes.Buffer
// 	old := log.Writer()
// 	log.SetOutput(&buf)
// 	t.Cleanup(func() { log.SetOutput(old) })

// 	done := make(chan struct{})
// 	go func() {
// 		runSession("s-ctx", "/bin/true", nil) // will block until ctx.Done()
// 		close(done)
// 	}()

// 	// Give Run() time to set ready, then signal the process (NotifyContext listens to SIGTERM/INT)
// 	time.Sleep(20 * time.Millisecond)
// 	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)

// 	select {
// 	case <-done:
// 	case <-time.After(2 * time.Second):
// 		t.Fatal("timeout waiting for runSession to return after SIGTERM")
// 	}

// 	if !bytes.Contains(buf.Bytes(), []byte("context closed")) {
// 		t.Fatalf("expected context-closed log; got: %s", buf.String())
// 	}
// 	if fc == nil || !fc.waitClose {
// 		t.Fatalf("expected WaitClose to be called on controller")
// 	}
// }

// func TestRootCmd_RunsWithFlags(t *testing.T) {
// 	orig := newSessionController
// 	// var captured *api.SessionSpec
// 	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
// 		// Use capturing ctrl to record spec and terminate via exit
// 		return &struct {
// 			*capturingCtrl
// 		}{newCapturingCtrl(ctx, exit).(*capturingCtrl)}
// 	}
// 	t.Cleanup(func() { newSessionController = orig })

// 	// Capture logs (to avoid noisy test output)
// 	var buf bytes.Buffer
// 	old := log.Writer()
// 	log.SetOutput(&buf)
// 	t.Cleanup(func() { log.SetOutput(old) })

// 	// Ensure global flags start clean for the test
// 	sessionID, sessionCmd = "", ""

// 	// Provide flags so we avoid random ID and default command paths
// 	rootCmd.SetArgs([]string{"--id", "s-flags", "--command", "/bin/sh"})

// 	// Hook to grab the spec: swap constructor that stores last instance
// 	var last *capturingCtrl
// 	newSessionController = func(ctx context.Context, exit chan error) api.SessionController {
// 		last = newCapturingCtrl(ctx, exit).(*capturingCtrl)
// 		return last
// 	}

// 	if err := rootCmd.Execute(); err != nil {
// 		t.Fatalf("rootCmd.Execute failed: %v", err)
// 	}

// 	if last == nil || last.addedSpec == nil {
// 		t.Fatalf("controller not constructed or AddSession not called")
// 	}

// 	got := *last.addedSpec
// 	if string(got.ID) != "s-flags" {
// 		t.Errorf("ID mismatch: got %q want %q", got.ID, "s-flags")
// 	}
// 	if got.Command != "/bin/sh" {
// 		t.Errorf("Command mismatch: got %q want %q", got.Command, "/bin/sh")
// 	}
// 	if got.Kind != api.SessLocal || got.Label != "default" {
// 		t.Errorf("unexpected spec fields: %+v", got)
// 	}
// 	if got.Env == nil || reflect.ValueOf(got.Env).Len() == 0 {
// 		t.Errorf("expected Env to be populated")
// 	}
// 	if got.CommandArgs == nil || len(got.CommandArgs) != 0 {
// 		t.Errorf("expected empty CommandArgs; got %v", got.CommandArgs)
// 	}
// }
