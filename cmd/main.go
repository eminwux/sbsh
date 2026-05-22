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
	"io"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/cmd/sb"
	"github.com/eminwux/sbsh/cmd/sbsh"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
)

type rootFactory func() (*cobra.Command, error)

func execRoot(root *cobra.Command) int {
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

func runWithFactory(ctx context.Context, factory rootFactory) int {
	root, err := factory()
	if err != nil {
		return 1
	}

	root.SetContext(ctx)
	return execRoot(root)
}

// dispatch selects the subtree to run based on the executable name (exe),
// falling back to the SBSH_DEBUG_MODE value (debug) when exe is unrecognized.
//
// The debug fallback is useful for development and debugging purposes: it
// allows running sbsh/sb even when the executable is not named sbsh or sb,
// e.g. when launched from an IDE or a debugger. SBSH_DEBUG_MODE can be set to
// "sbsh" or "sb" to run the corresponding subtree. The bool is false when
// neither exe nor debug names a known subtree.
func dispatch(exe, debug string) (rootFactory, bool) {
	factories := map[string]rootFactory{
		"sb":    sb.NewSbRootCmd,
		"sbsh":  sbsh.NewSbshRootCmd,
		"-sbsh": sbsh.NewSbshRootCmd,
	}

	if factory, ok := factories[exe]; ok {
		return factory, true
	}
	if factory, ok := factories[debug]; ok {
		return factory, true
	}
	return nil, false
}

func run(args []string, getenv func(string) string, stderr io.Writer) int {
	logger := logging.NewNoopLogger()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)

	exe := filepath.Base(args[0])
	debug := getenv("SBSH_DEBUG_MODE")

	factory, ok := dispatch(exe, debug)
	if !ok {
		fmt.Fprintf(stderr, "unknown entry command: %s\n", exe)
		return 1
	}
	return runWithFactory(ctx, factory)
}

func main() {
	os.Exit(run(os.Args, os.Getenv, os.Stderr))
}
