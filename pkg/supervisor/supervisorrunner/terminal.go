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

package supervisorrunner

import (
	"log"
	"net"
	"os"

	"golang.org/x/term"
)

// toBashUIMode: set terminal to RAW, update flags
func (sr *SupervisorRunnerExec) toBashUIMode() error {
	lastTermState, err := toRawMode()
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
		return err
	}
	// defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	sr.uiMode = UIBash
	sr.lastTermState = lastTermState
	return nil
}

// toSupervisorUIMode: set terminal to COOKED for your REPL
func (sr *SupervisorRunnerExec) toExitShell() error {
	if sr.lastTermState != nil {
		err := term.Restore(int(os.Stdin.Fd()), sr.lastTermState)
		if err != nil {
			log.Fatalf("MakeRaw: %v", err)
			return err
		}
	}

	sr.uiMode = UIExitShell
	return nil
}

func (sr *SupervisorRunnerExec) initTerminal(conn net.Conn) error {
	if err := writeterminal(conn, "export SBSH_SUP_SOCKET="+sr.supervisorSocketCtrl+"\n"); err != nil {
		return err
	}

	return nil

}

func writeterminal(conn net.Conn, input string) error {
	_, err := conn.Write([]byte(input))

	return err
}

func toRawMode() (*term.State, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("[supervisor] MakeRaw terminal: %v", err)

	}

	return state, nil
}
