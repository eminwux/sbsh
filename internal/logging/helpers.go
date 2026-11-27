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

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/pkg/api"
	publog "github.com/eminwux/sbsh/pkg/logging"
	"github.com/spf13/cobra"
)

// ReformatHandler aliases the public type so existing CLI references
// (e.g. cmd/sb/sb.go) keep compiling without an import rewrite.
type ReformatHandler = publog.ReformatHandler

// ParseLevel re-exports the public ParseLevel for the same reason.
func ParseLevel(lvl string) slog.Level { return publog.ParseLevel(lvl) }

// SetupFileLogger is SetupLogger for the file output. Kept for callers that
// only ever log to a file (the sbsh client supervisor).
func SetupFileLogger(cmd *cobra.Command, logfile string, loglevel string) error {
	return SetupLogger(cmd, api.LogOutputFile, logfile, loglevel)
}

// SetupLogger builds the terminal/client logger for output ("" / "file" or
// "stdout") and stores the logger, its LevelVar and its handler in cmd's
// context. "file" requires logfile; "stdout" requires logfile to be empty
// and creates no file.
func SetupLogger(cmd *cobra.Command, output, logfile, loglevel string) error {
	lo, errOut := api.NormalizeLogOutput(output)
	if errOut != nil {
		return errOut
	}
	var (
		fl  *publog.FileLogger
		err error
	)
	switch lo {
	case api.LogOutputStdout:
		if cmd == nil || loglevel == "" || logfile != "" {
			return fmt.Errorf(
				"cmd and loglevel=%s must not be empty, and logfile=%s must be empty for stdout output",
				loglevel, logfile,
			)
		}
		fl, err = publog.NewStdoutLogger(loglevel)
	default:
		if cmd == nil || logfile == "" || loglevel == "" {
			return fmt.Errorf("cmd, logfile=%s, loglevel=%s must not be empty", logfile, loglevel)
		}
		fl, err = publog.NewFileLogger(logfile, loglevel)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set up %s logger: %v\n", lo, err)
		return err
	}

	levelVar := fl.LevelVar
	ctx := cmd.Context()
	ctx = context.WithValue(ctx, types.CtxLogger, fl.Logger)
	ctx = context.WithValue(ctx, types.CtxLevelVar, &levelVar)
	ctx = context.WithValue(ctx, types.CtxHandler, fl.Handler)
	cmd.SetContext(ctx)
	return nil
}
