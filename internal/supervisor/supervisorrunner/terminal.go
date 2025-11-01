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
	"log/slog"
	"os"
	"time"

	"golang.org/x/term"
)

// toBashUIMode: set terminal to RAW, update flags.
func (sr *Exec) toBashUIMode() error {
	sr.logger.Debug("toBashUIMode: switching to raw mode")
	lastTermState, err := toRawMode(sr.logger)
	if err != nil {
		sr.logger.Error("toBashUIMode: failed to set raw mode", "error", err)
		return err
	}
	// defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	sr.uiMode = UIBash
	sr.lastTermState = lastTermState
	sr.logger.Info("toBashUIMode: switched to bash UI mode")
	return nil
}

// toSupervisorUIMode: set terminal to COOKED for your REPL.
func (sr *Exec) toExitShell() error {
	sr.logger.Debug("toExitShell: switching to cooked mode")
	if sr.lastTermState != nil {
		err := term.Restore(int(os.Stdin.Fd()), sr.lastTermState)
		if err != nil {
			sr.logger.Error("toExitShell: failed to restore terminal state", "error", err)
			return err
		}
	}

	sr.uiMode = UIExitShell
	sr.logger.Info("toExitShell: switched to exit shell UI mode")
	return nil
}

func (sr *Exec) initTerminal() error {
	sr.logger.Debug("initTerminal: supervisor writing to terminal")

	sr.logger.Info("initTerminal: terminal initialized")
	return nil
}

func (sr *Exec) writeTerminal(input string) error {
	sr.logger.Debug("writeTerminal: writing to terminal", "input_len", len(input))
	const maxRetries = 10
	const retrySleepMs = 100

	for i := range len(input) {
		var err error
		for attempt := range maxRetries {
			_, err = sr.ioConn.Write([]byte{input[i]})
			if err == nil {
				break
			}
			sr.logger.Warn("writeTerminal: write failed, retrying", "index", i, "attempt", attempt+1, "error", err)
			time.Sleep(retrySleepMs * time.Millisecond)
		}
		if err != nil {
			sr.logger.Error("writeTerminal: failed to write byte after retries", "index", i, "error", err)
			return err
		}
		time.Sleep(time.Microsecond)
	}
	sr.logger.Info("writeTerminal: finished writing to terminal")
	return nil
}

func toRawMode(logger *slog.Logger) (*term.State, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		logger.Error("toRawMode: failed to set raw mode", "error", err)
		return nil, err
	}
	logger.Info("toRawMode: terminal set to raw mode")
	return state, nil
}
