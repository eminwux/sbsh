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

package server

import (
	"errors"
	"fmt"

	"github.com/eminwux/sbsh/pkg/api"
)

// rpcAdapter satisfies api.TerminalController for the RPC layer by
// forwarding to the Server's runner. The Server's public Stop signature
// takes a Go error rather than *api.StopArgs, so the adapter is needed
// to bridge those two shapes without polluting the public surface.
type rpcAdapter struct {
	srv *Server
}

func (a *rpcAdapter) Run(_ *api.TerminalSpec) error {
	return errors.New("server: Run not implemented on facade; use Serve")
}

func (a *rpcAdapter) WaitReady() error { return nil }
func (a *rpcAdapter) WaitClose() error { return nil }

func (a *rpcAdapter) Close(reason error) error { return a.srv.Stop(reason) }

func (a *rpcAdapter) Ping(in *api.PingMessage) (*api.PingMessage, error) {
	if in != nil && in.Message == "PING" {
		return &api.PingMessage{Message: "PONG"}, nil
	}
	msg := ""
	if in != nil {
		msg = in.Message
	}
	return &api.PingMessage{}, fmt.Errorf("unexpected ping message: %s", msg)
}

func (a *rpcAdapter) Resize(args api.ResizeArgs) {
	if r := a.srv.getRunner(); r != nil {
		r.Resize(args)
	}
}

func (a *rpcAdapter) Detach(id *api.ID) error {
	r := a.srv.getRunner()
	if r == nil {
		return errors.New("server: terminal not running")
	}
	return r.Detach(id)
}

func (a *rpcAdapter) Attach(id *api.ID, response *api.ResponseWithFD) error {
	r := a.srv.getRunner()
	if r == nil {
		return errors.New("server: terminal not running")
	}
	if err := r.Attach(id, response); err != nil {
		return err
	}
	return r.PostAttachShell()
}

func (a *rpcAdapter) Metadata() (*api.TerminalDoc, error) {
	return a.srv.Metadata()
}

func (a *rpcAdapter) State() (*api.TerminalStatusMode, error) {
	md, err := a.srv.Metadata()
	if err != nil {
		return nil, err
	}
	return &md.Status.State, nil
}

func (a *rpcAdapter) Stop(args *api.StopArgs) error {
	reason := "stop requested"
	if args != nil && args.Reason != "" {
		reason = args.Reason
	}
	// Trigger shutdown asynchronously so the RPC reply lands before the
	// server socket is torn down — matches Controller.Stop semantics.
	go func() { _ = a.srv.Stop(fmt.Errorf("stop: %s", reason)) }()
	return nil
}

func (a *rpcAdapter) Write(req *api.WriteRequest) error {
	r := a.srv.getRunner()
	if r == nil {
		return errors.New("server: terminal not running")
	}
	if req == nil || len(req.Data) == 0 {
		return nil
	}
	return r.WritePTY(req.Data)
}

func (a *rpcAdapter) Subscribe(req *api.SubscribeRequest, response *api.ResponseWithFD) error {
	r := a.srv.getRunner()
	if r == nil {
		return errors.New("server: terminal not running")
	}
	return r.Subscribe(req, response)
}

func (a *rpcAdapter) Screenshot(args *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
	r := a.srv.getRunner()
	if r == nil {
		return nil, errors.New("server: terminal not running")
	}
	return r.Screenshot(args)
}
