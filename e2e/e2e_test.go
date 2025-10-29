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

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/creack/pty"
)

const (
	sbsh = "sbsh"
	sb   = "sb"
)

// runReturningBinary runs the provided binary with args, fails the test on non-zero exit or empty output.
// If the binary file does not exist, the test is skipped.
func runReturningBinary(t *testing.T, command string, args ...string) []byte {
	t.Helper()

	dir := os.Getenv("E2E_BIN_DIR")
	if dir == "" {
		dir = ".." // or detect repo root
	}
	bin := filepath.Join(dir, command)

	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("binary %s not found, skipping", bin)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running %s %v failed: %v\noutput:\n%s", bin, args, err, string(out))
	}
	if len(out) == 0 {
		t.Fatalf("no output from %s %v", bin, args)
	}

	return out
}

func runPersistentBinaryPty(
	t *testing.T,
	fdOut *os.File,
	waitErr func(*os.ProcessState, error),
	command string,
	args ...string,
) {
	t.Helper()

	dir := os.Getenv("E2E_BIN_DIR")
	if dir == "" {
		dir = ".." // or detect repo root
	}
	bin := filepath.Join(dir, command)

	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("binary %s not found, skipping", bin)
	}

	env := append(os.Environ(),
		`PS1="__P__ "`,
		"TERM=xterm",
		"LANG=C",
		"COLUMNS=120",
		"LINES=40",
	)
	// Files slice maps directly: index -> child fd number.
	files := []*os.File{
		fdOut, // child fd 0 (stdin)
		fdOut, // child fd 1 (stdout)
		fdOut, // child fd 2 (stderr) -> same FD as stdout
	}

	procAttr := &os.ProcAttr{
		Env:   env,
		Files: files,
		Sys: &syscall.SysProcAttr{
			Setsid:  true, // detach from your pg/ctty
			Setctty: true,
			Ctty:    0,
		},
	}

	p, err := os.StartProcess(bin, append([]string{bin}, args...), procAttr)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	// Parent can close fds it doesn't use:
	fdOut.Close()

	go func(p *os.Process) {
		waitErr(p.Wait())
	}(p)
}

func TestSbsh_Default(t *testing.T) {
	t.Cleanup(func() {
		runReturningBinary(t, sb, "prune", "terminals")
		runReturningBinary(t, sb, "prune", "supervisors")
	})

	// Open a pty
	ptmx, pts, errOpen := pty.Open()
	if errOpen != nil {
		t.Errorf("error opening pty: %v", errOpen)
	}

	// Copy cleanup function
	defer func() {
		_ = ptmx.Close()
	}()

	// Set initial size
	errSize := pty.Setsize(ptmx, &pty.Winsize{Cols: 120, Rows: 40})
	if errSize != nil {
		t.Errorf("error setting pty size: %v", errSize)
	}

	ret := make(chan error, 1)
	cmdWaitFunc := func(_ *os.ProcessState, err error) {
		ret <- func(err error) error {
			if err != nil {
				t.Log(
					"process exited with error",
					"error",
					err,
				)
				return err
			}
			t.Log("process exited with empty error")
			return nil
		}(err)
	}
	runPersistentBinaryPty(t, pts, cmdWaitFunc, sbsh, "--session-id", "customId")

	////////////////////////////////////////////////////////
	time.Sleep(1 * time.Second) // wait for sbsh to initialize

	// Cancel: close ptmx when ctx is done → unblocks Read.
	go func() {
		<-t.Context().Done()
		_ = ptmx.Close() // Read returns with error
	}()

	// Create expect console
	console, err := expect.NewConsole(
		expect.WithStdin(ptmx),
		expect.WithCloser(ptmx),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()

	go func() {
		buf := make([]byte, 4096)

		for {
			n, errR := console.Tty().Read(buf) // blocks until data or close
			if n > 0 {
				t.Logf("sbsh output: %q", buf[:n])
			}
			if n == 0 {
				t.Logf("reader read 0 bytes, exiting")
			}
			if errR != nil {
				// io.EOF / use-after-close is expected on cancel
				t.Logf("reader exit: %v", errR)
				return
			}
		}
	}()

	_, errS := console.SendLine("echo hello")
	if errS != nil {
		t.Fatalf("error sending to sbsh: %v", errS)
	}

	// Expect the output
	re := regexp.MustCompile(`.*\[sbsh-customId\].*`)
	response, errE := console.Expect(
		expect.WithTimeout(1*time.Second),
		expect.Regexp(re),
	)
	if errE != nil {
		t.Fatalf("did not see sbsh output: %v", errE)
	}

	t.Logf("received sbsh output: %q", response)

	ptmx.WriteString("\x04") // send EOF to sbsh)

	// Wait for process to exit and get error
	<-ret
}
