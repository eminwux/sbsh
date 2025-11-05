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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/cmd/sb"
	"github.com/eminwux/sbsh/cmd/sbsh"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
)

func execRoot(root *cobra.Command) int {
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

func main() {
	logger := logging.NewNoopLogger()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	// Select which subtree to run based on the executable name
	exe := filepath.Base(os.Args[0])
	// Decide which subtree to run by prepending the name as the first arg
	switch exe {
	case "sb":
		cmd, err := sb.NewSbRootCmd()
		cmd.SetContext(ctx)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(execRoot(cmd)) // <- sbsh IS the root; no wrapper

	case "sbsh", "-sbsh":
		cmd, err := sbsh.NewSbshRootCmd()
		cmd.SetContext(ctx)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(execRoot(cmd)) // <- sbsh IS the root; no wrapper

	default:
		// Fallback to sbsh if SBSH_DEBUG_MODE is set
		// This is useful for development and debugging purposes
		// as it allows to run sbsh even if the executable is not named sbsh
		// or sb. For example, when running from an IDE or a debugger.
		// In this case, SBSH_DEBUG_MODE can be set to "sbsh" or "sb"
		// to run the corresponding subtree.
		// If SBSH_DEBUG_MODE is not set, an error is printed and the program exits.
		// This avoids confusion and ensures that the user is aware of the correct usage.
		debug := os.Getenv("SBSH_DEBUG_MODE")
		if debug == "" {
			fmt.Fprintf(os.Stderr, "unknown entry command: %s\n", exe)
			os.Exit(1)
		}
		switch os.Getenv("SBSH_DEBUG_MODE") {
		case "sb":
			cmd, err := sbsh.NewSbshRootCmd()
			cmd.SetContext(ctx)
			if err != nil {
				os.Exit(1)
			}
			os.Exit(execRoot(cmd))
		case "sbsh", "-sbsh":
			cmd, err := sbsh.NewSbshRootCmd()
			cmd.SetContext(ctx)
			if err != nil {
				os.Exit(1)
			}
			os.Exit(execRoot(cmd))
		default:
			fmt.Fprintf(os.Stderr, "unknown entry command: %s\n", exe)
			os.Exit(1)
		}
	}
}
