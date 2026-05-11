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
	publog "github.com/eminwux/sbsh/pkg/logging"
	"github.com/spf13/cobra"
)

// ReformatHandler aliases the public type so existing CLI references
// (e.g. cmd/sb/sb.go) keep compiling without an import rewrite.
type ReformatHandler = publog.ReformatHandler

// ParseLevel re-exports the public ParseLevel for the same reason.
func ParseLevel(lvl string) slog.Level { return publog.ParseLevel(lvl) }

func SetupFileLogger(cmd *cobra.Command, logfile string, loglevel string) error {
	if cmd == nil || logfile == "" || loglevel == "" {
		return fmt.Errorf("cmd, logfile=%s, loglevel=%s must not be empty", logfile, loglevel)
	}
	fl, err := publog.NewFileLogger(logfile, loglevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set up file logger: %v\n", err)
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
