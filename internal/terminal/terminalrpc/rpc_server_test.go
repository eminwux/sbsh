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

package terminalrpc

import (
	"errors"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

var errCore = errors.New("core failure")

// fakeController is a configurable api.TerminalController stub. Each method
// records its argument and returns the preconfigured value/error so the thin
// TerminalControllerRPC wrappers can be exercised on both their success and
// error branches without a live runtime.
type fakeController struct {
	pingResp   *api.PingMessage
	pingErr    error
	resizeArgs api.ResizeArgs
	detachID   *api.ID
	detachErr  error
	attachErr  error
	metadata   *api.TerminalDoc
	metaErr    error
	state      *api.TerminalStatusMode
	stateErr   error
	stopArgs   *api.StopArgs
	stopErr    error
	writeReq   *api.WriteRequest
	writeErr   error
	subErr     error
	shotResult *api.ScreenshotResult
	shotErr    error
}

func (f *fakeController) Run(*api.TerminalSpec) error { return nil }
func (f *fakeController) WaitReady() error            { return nil }
func (f *fakeController) WaitClose() error            { return nil }
func (f *fakeController) Close(error) error           { return nil }

func (f *fakeController) Ping(*api.PingMessage) (*api.PingMessage, error) {
	return f.pingResp, f.pingErr
}

func (f *fakeController) Resize(args api.ResizeArgs) { f.resizeArgs = args }

func (f *fakeController) Detach(id *api.ID) error {
	f.detachID = id
	return f.detachErr
}

func (f *fakeController) Attach(*api.AttachRequest, *api.ResponseWithFD) error {
	return f.attachErr
}

func (f *fakeController) Metadata() (*api.TerminalDoc, error) {
	return f.metadata, f.metaErr
}

func (f *fakeController) State() (*api.TerminalStatusMode, error) {
	return f.state, f.stateErr
}

func (f *fakeController) Stop(args *api.StopArgs) error {
	f.stopArgs = args
	return f.stopErr
}

func (f *fakeController) Write(req *api.WriteRequest) error {
	f.writeReq = req
	return f.writeErr
}

func (f *fakeController) Subscribe(*api.SubscribeRequest, *api.ResponseWithFD) error {
	return f.subErr
}

func (f *fakeController) Screenshot(*api.ScreenshotArgs) (*api.ScreenshotResult, error) {
	return f.shotResult, f.shotErr
}

func TestPing_CopiesPongOnSuccess(t *testing.T) {
	core := &fakeController{pingResp: &api.PingMessage{Message: "pong"}}
	s := &TerminalControllerRPC{Core: core}
	var out api.PingMessage
	if err := s.Ping(&api.PingMessage{Message: "ping"}, &out); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if out.Message != "pong" {
		t.Fatalf("out.Message = %q; want pong", out.Message)
	}
}

func TestPing_PropagatesError(t *testing.T) {
	s := &TerminalControllerRPC{Core: &fakeController{pingErr: errCore}}
	var out api.PingMessage
	if err := s.Ping(&api.PingMessage{}, &out); !errors.Is(err, errCore) {
		t.Fatalf("Ping err = %v; want errCore", err)
	}
}

func TestResize_DelegatesArgs(t *testing.T) {
	core := &fakeController{}
	s := &TerminalControllerRPC{Core: core}
	if err := s.Resize(api.ResizeArgs{Cols: 80, Rows: 24}, &api.Empty{}); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if core.resizeArgs.Cols != 80 || core.resizeArgs.Rows != 24 {
		t.Fatalf("resizeArgs = %+v; want {80 24}", core.resizeArgs)
	}
}

func TestDetach_Delegates(t *testing.T) {
	core := &fakeController{}
	s := &TerminalControllerRPC{Core: core}
	id := api.ID("term-1")
	if err := s.Detach(&id, &api.Empty{}); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if core.detachID == nil || *core.detachID != id {
		t.Fatalf("detachID = %v; want %v", core.detachID, id)
	}

	s2 := &TerminalControllerRPC{Core: &fakeController{detachErr: errCore}}
	if err := s2.Detach(&id, &api.Empty{}); !errors.Is(err, errCore) {
		t.Fatalf("Detach err = %v; want errCore", err)
	}
}

func TestAttach_Delegates(t *testing.T) {
	s := &TerminalControllerRPC{Core: &fakeController{}}
	if err := s.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	s2 := &TerminalControllerRPC{Core: &fakeController{attachErr: errCore}}
	if err := s2.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, errCore) {
		t.Fatalf("Attach err = %v; want errCore", err)
	}
}

func TestMetadata_CopiesDocOnSuccess(t *testing.T) {
	core := &fakeController{metadata: &api.TerminalDoc{Kind: api.Kind("Terminal")}}
	s := &TerminalControllerRPC{Core: core}
	var out api.TerminalDoc
	if err := s.Metadata(api.Empty{}, &out); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if out.Kind != api.Kind("Terminal") {
		t.Fatalf("out.Kind = %q; want Terminal", out.Kind)
	}

	s2 := &TerminalControllerRPC{Core: &fakeController{metaErr: errCore}}
	if err := s2.Metadata(api.Empty{}, &out); !errors.Is(err, errCore) {
		t.Fatalf("Metadata err = %v; want errCore", err)
	}
}

func TestState_CopiesStateOnSuccess(t *testing.T) {
	want := api.Ready
	core := &fakeController{state: &want}
	s := &TerminalControllerRPC{Core: core}
	var out api.TerminalStatusMode
	if err := s.State(api.Empty{}, &out); err != nil {
		t.Fatalf("State: %v", err)
	}
	if out != api.Ready {
		t.Fatalf("out = %v; want Ready", out)
	}

	s2 := &TerminalControllerRPC{Core: &fakeController{stateErr: errCore}}
	if err := s2.State(api.Empty{}, &out); !errors.Is(err, errCore) {
		t.Fatalf("State err = %v; want errCore", err)
	}
}

func TestStop_Delegates(t *testing.T) {
	core := &fakeController{}
	s := &TerminalControllerRPC{Core: core}
	if err := s.Stop(&api.StopArgs{Reason: "bye"}, &api.Empty{}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if core.stopArgs == nil || core.stopArgs.Reason != "bye" {
		t.Fatalf("stopArgs = %+v; want reason bye", core.stopArgs)
	}

	s2 := &TerminalControllerRPC{Core: &fakeController{stopErr: errCore}}
	if err := s2.Stop(&api.StopArgs{}, &api.Empty{}); !errors.Is(err, errCore) {
		t.Fatalf("Stop err = %v; want errCore", err)
	}
}

func TestWrite_Delegates(t *testing.T) {
	core := &fakeController{}
	s := &TerminalControllerRPC{Core: core}
	if err := s.Write(&api.WriteRequest{Data: []byte("hi")}, &api.Empty{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if core.writeReq == nil || string(core.writeReq.Data) != "hi" {
		t.Fatalf("writeReq = %+v; want data hi", core.writeReq)
	}

	s2 := &TerminalControllerRPC{Core: &fakeController{writeErr: errCore}}
	if err := s2.Write(&api.WriteRequest{}, &api.Empty{}); !errors.Is(err, errCore) {
		t.Fatalf("Write err = %v; want errCore", err)
	}
}

func TestSubscribe_Delegates(t *testing.T) {
	s := &TerminalControllerRPC{Core: &fakeController{}}
	if err := s.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	s2 := &TerminalControllerRPC{Core: &fakeController{subErr: errCore}}
	if err := s2.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); !errors.Is(err, errCore) {
		t.Fatalf("Subscribe err = %v; want errCore", err)
	}
}

func TestScreenshot_CopiesResultOnSuccess(t *testing.T) {
	core := &fakeController{shotResult: &api.ScreenshotResult{Cols: 80, Rows: 24, Text: "grid"}}
	s := &TerminalControllerRPC{Core: core}
	var out api.ScreenshotResult
	if err := s.Screenshot(&api.ScreenshotArgs{}, &out); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if out.Cols != 80 || out.Text != "grid" {
		t.Fatalf("out = %+v; want cols 80 text grid", out)
	}

	s2 := &TerminalControllerRPC{Core: &fakeController{shotErr: errCore}}
	if err := s2.Screenshot(&api.ScreenshotArgs{}, &out); !errors.Is(err, errCore) {
		t.Fatalf("Screenshot err = %v; want errCore", err)
	}
}
