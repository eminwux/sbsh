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

package stop

import (
	"context"
	"errors"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/viper"
)

func Test_ErrLoggerNotFound_Terminal_RunE(t *testing.T) {
	cmd := NewStopTerminalsCmd()
	ctx := context.Background()
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{"foo"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ResolveStopOpts(t *testing.T) {
	type tc struct {
		name    string
		args    []string
		setup   func()
		wantErr error
	}
	cases := []tc{
		{
			name:    "no name and not --all errors",
			args:    []string{},
			setup:   func() {},
			wantErr: errdefs.ErrNoTerminalIdentifier,
		},
		{
			name: "name with --all errors",
			args: []string{"foo"},
			setup: func() {
				viper.Set(config.SB_STOP_ALL.ViperKey, true)
			},
			wantErr: errdefs.ErrInvalidFlag,
		},
		{
			name: "zero timeout errors",
			args: []string{"foo"},
			setup: func() {
				viper.Set(config.SB_STOP_TIMEOUT.ViperKey, 0)
			},
			wantErr: errdefs.ErrInvalidFlag,
		},
		{
			name: "name only succeeds",
			args: []string{"foo"},
			setup: func() {
				viper.Set(config.SB_STOP_TIMEOUT.ViperKey, "5s")
			},
			wantErr: nil,
		},
		{
			name: "--all with no name succeeds",
			args: []string{},
			setup: func() {
				viper.Set(config.SB_STOP_ALL.ViperKey, true)
				viper.Set(config.SB_STOP_TIMEOUT.ViperKey, "5s")
			},
			wantErr: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			viper.Reset()
			c.setup()
			cmd := NewStopTerminalsCmd()
			_, err := resolveStopOpts(cmd, c.args)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("expected %v, got %v", c.wantErr, err)
			}
		})
	}
}

func Test_ProcessAlive_InvalidPid(t *testing.T) {
	if processAlive(0) {
		t.Fatal("expected pid 0 to be reported dead")
	}
	if processAlive(-1) {
		t.Fatal("expected pid -1 to be reported dead")
	}
}
