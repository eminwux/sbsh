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

package builder

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/pkg/api"
)

// ClientOption mutates a clientConfig. Later options override
// earlier ones so callers can compose defaults + overrides.
type ClientOption func(*clientConfig)

// clientConfig accumulates ClientOption values. Nil-able fields
// distinguish "not set" from zero values (e.g. detachKeystroke).
type clientConfig struct {
	id              string
	name            string
	logFile         string
	socketFile      string
	mode            api.ClientMode
	modeSet         bool
	detachKeystroke *bool
	terminalSpec    *api.TerminalSpec
}

// WithClientID sets the client ID. Empty means "generate a random
// ID at build time".
func WithClientID(id string) ClientOption {
	return func(c *clientConfig) { c.id = id }
}

// WithClientName sets the human-readable client name. Empty means
// "generate a random name at build time".
func WithClientName(name string) ClientOption {
	return func(c *clientConfig) { c.name = name }
}

// WithClientLogFile overrides the client's log file path. Empty
// leaves the field unset (the client process picks its own
// default).
func WithClientLogFile(path string) ClientOption {
	return func(c *clientConfig) { c.logFile = path }
}

// WithClientSocketFile overrides the client's control-socket path.
// Empty means "derive from runPath + client ID".
func WithClientSocketFile(path string) ClientOption {
	return func(c *clientConfig) { c.socketFile = path }
}

// WithClientMode selects between RunNewTerminal (default) and
// AttachToTerminal. Calling this explicitly is required for
// attach-style clients.
func WithClientMode(mode api.ClientMode) ClientOption {
	return func(c *clientConfig) {
		c.mode = mode
		c.modeSet = true
	}
}

// WithClientDetachKeystroke enables or disables the ^] ^] detach
// keystroke. The default is enabled (true). Setting false matches
// the CLI's --disable-detach behavior.
func WithClientDetachKeystroke(enable bool) ClientOption {
	return func(c *clientConfig) {
		v := enable
		c.detachKeystroke = &v
	}
}

// WithClientTerminalSpec embeds a pre-built TerminalSpec into the
// ClientDoc. For RunNewTerminal clients this is the spec that will
// be spawned; for AttachToTerminal clients callers typically pass
// a spec carrying only ID or Name (the controller resolves the
// concrete terminal at run time). A nil spec is normalized to an
// empty &api.TerminalSpec{} so downstream code never has to guard
// against nil.
func WithClientTerminalSpec(spec *api.TerminalSpec) ClientOption {
	return func(c *clientConfig) { c.terminalSpec = spec }
}

// BuildClientDoc produces a ClientDoc from a runPath plus optional
// inline overrides. Defaults mirror the CLI: random ID/Name when
// unset, socket path under runPath/clients/<id>/socket, detach
// keystroke enabled, RunNewTerminal mode.
//
// runPath is required; an empty runPath returns
// errdefs.ErrRunPathRequired.
//
// ctx and logger are accepted but unused today; they are reserved
// for future profile/discovery plumbing so the signature matches
// BuildTerminalSpec and will not have to change when that lands.
func BuildClientDoc(
	_ context.Context,
	_ *slog.Logger,
	runPath string,
	opts ...ClientOption,
) (*api.ClientDoc, error) {
	if runPath == "" {
		return nil, errdefs.ErrRunPathRequired
	}

	cfg := clientConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if cfg.id == "" {
		cfg.id = naming.RandomID()
	}
	if cfg.name == "" {
		cfg.name = naming.RandomName()
	}
	if cfg.socketFile == "" {
		cfg.socketFile = filepath.Join(runPath, defaults.ClientsRunPath, cfg.id, "socket")
	}
	if cfg.terminalSpec == nil {
		cfg.terminalSpec = &api.TerminalSpec{}
	}

	detach := true
	if cfg.detachKeystroke != nil {
		detach = *cfg.detachKeystroke
	}

	mode := api.RunNewTerminal
	if cfg.modeSet {
		mode = cfg.mode
	}

	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        cfg.name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: api.ClientSpec{
			ID:              api.ID(cfg.id),
			RunPath:         runPath,
			LogFile:         cfg.logFile,
			SockerCtrl:      cfg.socketFile,
			TerminalSpec:    cfg.terminalSpec,
			DetachKeystroke: detach,
			ClientMode:      mode,
		},
	}
	return doc, nil
}
