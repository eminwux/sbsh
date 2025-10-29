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
	"os"
	"regexp"
	"testing"
	"time"
)

func TestSbsh_Help(t *testing.T) {
	// Make this test parallel-friendly with other tests
	t.Parallel()

	// Check sbsh binary help output
	_ = runReturningBinary(t, nil, sbsh, "-h")
	_ = runReturningBinary(t, nil, sbsh, "--help")
}

func TestSbsh_Default(t *testing.T) {
	runPath := getRandomRunPath(t)
	mkdirRunPath(t, runPath)

	t.Cleanup(func() {
		runPathEnv := buildSbRunPathEnv(t, runPath)
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "terminals", "--verbose", "--log-level", "debug")
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "supervisors", "--verbose", "--log-level", "debug")
	})

	// Setup pty
	ptmx, pts := setupPty(t)
	defer func() {
		_ = ptmx.Close()
	}()

	// Setup logging pipe
	pipeLogR, pipeLogW, errLogPipe := os.Pipe()
	if errLogPipe != nil {
		t.Fatalf("error creating pipe: %v", errLogPipe)
	}

	// Setup defer closes
	defer func() {
		_ = pipeLogR.Close()
		_ = pipeLogW.Close()
	}()

	// Setup multiwriter to log and expect
	multiW := setupMultiWriter(t, ptmx, pipeLogW)

	// Start logging goroutine
	go logTerminal(t, pipeLogR)

	// Setup expect pipe
	pipeExpectR, pipeExpectW, errPipeE := os.Pipe()
	if errPipeE != nil {
		t.Fatalf("error creating pipe: %v", errPipeE)
	}
	// Setup defer closes
	defer func() {
		_ = pipeExpectR.Close()
		_ = pipeExpectW.Close()
	}()

	// Setup prompt detector
	regex := regexp.MustCompile(`.*\(sbsh-.+\).*`)
	promptCh := setupPromptDetector(t, pipeExpectR, regex)

	// Add expect writer to multiwriter
	multiW.Add(pipeExpectW)

	// Run sbsh
	ret := make(chan error, 1)
	cmdWaitFunc := cmdWaitFunc(t, &ret)
	env := append([]string{}, getRandomSbshRunPath(t, runPath))
	runPersistentBinaryPty(t, pts, cmdWaitFunc, env, sbsh)

	// Wait for prompt detection
	<-promptCh

	// Remove expect writer from multiwriter
	multiW.Remove(pipeExpectW)

	// Give some time for sbsh to be ready to accept commands
	time.Sleep(100 * time.Millisecond)

	// Send EOF to sbsh
	ptmx.WriteString("\x04")

	// Wait for process to exit and get error
	select {
	case <-ret:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for process to exit")
	}
}

func TestSbsh_CustomId(t *testing.T) {
	runPath := getRandomRunPath(t)
	mkdirRunPath(t, runPath)

	t.Cleanup(func() {
		runPathEnv := buildSbRunPathEnv(t, runPath)
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "terminals", "--verbose", "--log-level", "debug")
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "supervisors", "--verbose", "--log-level", "debug")
	})

	// Setup pty
	ptmx, pts := setupPty(t)
	defer func() {
		_ = ptmx.Close()
	}()

	// Setup logging pipe
	pipeLogR, pipeLogW, errLogPipe := os.Pipe()
	if errLogPipe != nil {
		t.Fatalf("error creating pipe: %v", errLogPipe)
	}

	// Setup defer closes
	defer func() {
		_ = pipeLogR.Close()
		_ = pipeLogW.Close()
	}()

	// Setup multiwriter to log and expect
	multiW := setupMultiWriter(t, ptmx, pipeLogW)

	// Start logging goroutine
	go logTerminal(t, pipeLogR)

	// Setup expect pipe
	pipeExpectR, pipeExpectW, errPipeE := os.Pipe()
	if errPipeE != nil {
		t.Fatalf("error creating pipe: %v", errPipeE)
	}
	// Setup defer closes
	defer func() {
		_ = pipeExpectR.Close()
		_ = pipeExpectW.Close()
	}()

	// Setup prompt detector
	regex := regexp.MustCompile(`.*\(sbsh-customId\).*`)
	promptCh := setupPromptDetector(t, pipeExpectR, regex)

	// Add expect writer to multiwriter
	multiW.Add(pipeExpectW)

	// Run sbsh
	ret := make(chan error, 1)
	cmdWaitFunc := cmdWaitFunc(t, &ret)
	env := append([]string{}, getRandomSbshRunPath(t, runPath))
	runPersistentBinaryPty(t, pts, cmdWaitFunc, env, sbsh, "--session-id", "customId")

	// Wait for prompt detection
	<-promptCh

	// Remove expect writer from multiwriter
	multiW.Remove(pipeExpectW)

	// Give some time for sbsh to be ready to accept commands
	time.Sleep(100 * time.Millisecond)

	// Send EOF to sbsh
	ptmx.WriteString("\x04")

	// Wait for process to exit and get error
	select {
	case <-ret:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for process to exit")
	}
}

func TestSbsh_Detach(t *testing.T) {
	runPath := getRandomRunPath(t)
	mkdirRunPath(t, runPath)

	t.Cleanup(func() {
		runPathEnv := buildSbRunPathEnv(t, runPath)
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "terminals", "--verbose", "--log-level", "debug")
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "supervisors", "--verbose", "--log-level", "debug")
	})

	// Setup pty
	ptmx, pts := setupPty(t)
	defer func() {
		_ = ptmx.Close()
	}()

	// Setup logging pipe
	pipeLogR, pipeLogW, errLogPipe := os.Pipe()
	if errLogPipe != nil {
		t.Fatalf("error creating pipe: %v", errLogPipe)
	}

	// Setup defer closes
	defer func() {
		_ = pipeLogR.Close()
		_ = pipeLogW.Close()
	}()

	// Setup multiwriter to log and expect
	multiW := setupMultiWriter(t, ptmx, pipeLogW)

	// Start logging goroutine
	go logTerminal(t, pipeLogR)

	// Setup expect pipe
	pipeExpectR, pipeExpectW, errPipeE := os.Pipe()
	if errPipeE != nil {
		t.Fatalf("error creating pipe: %v", errPipeE)
	}
	// Setup defer closes
	defer func() {
		_ = pipeExpectR.Close()
		_ = pipeExpectW.Close()
	}()

	// Setup prompt detector
	promptCh := setupPromptDetector(t, pipeExpectR, nil)

	// Add expect writer to multiwriter
	multiW.Add(pipeExpectW)

	// Run sbsh
	ret := make(chan error, 1)
	cmdWaitFunc := cmdWaitFunc(t, &ret)
	env := append([]string{}, getRandomSbshRunPath(t, runPath))
	runPersistentBinaryPty(t, pts, cmdWaitFunc, env, sbsh)

	// Wait for prompt detection
	<-promptCh

	// Remove expect writer from multiwriter
	multiW.Remove(pipeExpectW)

	// Give some time for sbsh to be ready to accept commands
	time.Sleep(100 * time.Millisecond)

	// Send detach command
	ptmx.WriteString("sb detach\n")

	// Wait for process to exit and get error
	select {
	case <-ret:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for process to exit")
	}
}
