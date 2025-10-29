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
	"path"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/session/sessionrunner"
)

const (
	sbsh = "sbsh"
	sb   = "sb"
)

// runReturningBinary runs the provided binary with args, fails the test on non-zero exit or empty output.
// If the binary file does not exist, the test is skipped.
func runReturningBinary(t *testing.T, env []string, command string, args ...string) []byte {
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
	if env != nil {
		cmd.Env = env
	}

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
	env []string,
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

	env = append(env,
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

func setupPty(t *testing.T) (*os.File, *os.File) {
	t.Helper()

	// Open a pty
	ptmx, pts, errOpen := pty.Open()
	if errOpen != nil {
		t.Errorf("error opening pty: %v", errOpen)
	}

	// Set initial size
	errSize := pty.Setsize(ptmx, &pty.Winsize{Cols: 120, Rows: 40})
	if errSize != nil {
		t.Errorf("error setting pty size: %v", errSize)
	}

	// Cancel: close ptmx when ctx is done → unblocks Read.
	go func() {
		<-t.Context().Done()
		_ = ptmx.Close() // Read returns with error
	}()
	return ptmx, pts
}

func cmdWaitFunc(t *testing.T, ret *chan error) func(*os.ProcessState, error) {
	return func(_ *os.ProcessState, err error) {
		*ret <- func(err error) error {
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
}

func logTerminal(t *testing.T, r *os.File) {
	buf := make([]byte, 4096)

	for {
		n, errR := r.Read(buf) // blocks until data or close
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
}

func setupPromptDetector(t *testing.T, pipeExpectR *os.File, regex *regexp.Regexp) chan struct{} {
	t.Helper()

	if regex == nil {
		regex = regexp.MustCompile(`.*\(sbsh-.+\).*`)
	}

	promptCh := make(chan struct{}, 1)
	go func(promptCh chan struct{}) {
		buf := make([]byte, 1024)
		for {
			n, err := pipeExpectR.Read(buf)
			if n > 0 {
				matches := regex.FindSubmatch(buf[:n])
				if matches != nil {
					t.Logf("detected sbsh prompt: %q", matches[0])
					close(promptCh)
					return
				}
			}
			if n == 0 {
				return
			}
			if err != nil {
				return
			}
		}
	}(promptCh)
	return promptCh
}

func mkdirRunPath(t *testing.T, fullDir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working dir: %v", err)
	}
	fullDir = filepath.Join(cwd, fullDir)
	if err = os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatalf("could not create dir %s: %v", fullDir, err)
	}
}

func getRandomRunPath(t *testing.T) string {
	t.Helper()
	rndDir := naming.RandomID()
	fullDir := path.Join("tmp", rndDir)
	return fullDir
}

func buildSbRunPathEnv(t *testing.T, runPath string) string {
	t.Helper()
	return "SB_RUN_PATH=" + runPath
}

func getRandomSbshRunPath(t *testing.T, runPath string) string {
	t.Helper()
	return "SBSH_RUN_PATH=" + runPath
}

func setupMultiWriter(t *testing.T, ptmx *os.File, pipeLogW *os.File) *sessionrunner.DynamicMultiWriter {
	t.Helper()
	multiW := sessionrunner.NewDynamicMultiWriter(nil, pipeLogW)
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := ptmx.Read(buf)
			if n > 0 {
				_, _ = multiW.Write(buf[:n])
			}
			if n == 0 {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return multiW
}
