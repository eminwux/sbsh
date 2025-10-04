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

package session

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/rpc"
	"sbsh/pkg/common"
	"sync"

	"golang.org/x/sys/unix"
)

type unixJSONClientCodec struct {
	uc    *common.LoggingConnUnix
	encMu sync.Mutex
	enc   *json.Encoder

	// stash from last header read:
	pendingResult []byte
	pendingFDs    []int
	pendingSeq    uint64
	pendingErr    string
}

func newUnixJSONClientCodec(u *net.UnixConn) *unixJSONClientCodec {
	luc := &common.LoggingConnUnix{
		UnixConn:    u,
		PrefixWrite: "client->server",
		PrefixRead:  "server->client",
	}
	return &unixJSONClientCodec{
		uc:  luc,
		enc: json.NewEncoder(luc), // writes go through the logger
	}
}

func (c *unixJSONClientCodec) WriteRequest(req *rpc.Request, body any) error {
	c.encMu.Lock()
	defer c.encMu.Unlock()

	wire := struct {
		ID     uint64         `json:"id"`
		Method string         `json:"method"`
		Params [1]interface{} `json:"params"`
	}{ID: uint64(req.Seq), Method: req.ServiceMethod, Params: [1]any{body}}

	// Encode to buffer (so we can inspect it before sending)
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&wire); err != nil {
		return err
	}

	return c.enc.Encode(&wire)
}

func (c *unixJSONClientCodec) ReadResponseHeader(resp *rpc.Response) error {
	slog.Debug("[client-codec] Starting ReadResponseHeader")

	// Block using netpoller-aware ReadMsgUnix.
	buf := make([]byte, 64<<10) // 64KB frame buffer
	oob := make([]byte, 256)

	slog.Debug("[client-codec] Starting ReadMsgUnix")

	n, oobn, _, err := c.uc.ReadMsgUnix(buf, oob)
	if err != nil {
		return err
	}

	slog.Debug("[client-codec] Finished ReadMsgUnix")
	slog.Debug("[client-codec] Starting parsing FDs")

	// Parse any passed FDs.
	c.pendingFDs = c.pendingFDs[:0]
	if oobn > 0 {
		if cmsgs, _ := unix.ParseSocketControlMessage(oob[:oobn]); len(cmsgs) > 0 {
			for _, m := range cmsgs {
				if fds, _ := unix.ParseUnixRights(&m); len(fds) > 0 {
					c.pendingFDs = append(c.pendingFDs, fds...)
				}
			}
		}
	}

	slog.Debug("[client-codec] Done parsing FDs")
	slog.Debug("[client-codec] Starting JSON Unmarshalling")

	// Decode the JSON-RPC envelope to get id/error/result.
	var wire struct {
		ID     uint64           `json:"id"`
		Result *json.RawMessage `json:"result"`
		Error  *string          `json:"error"`
	}
	if err := json.Unmarshal(buf[:n], &wire); err != nil {
		return err
	}
	slog.Debug("[client-codec] Done JSON Unmarshalling")

	c.pendingSeq = wire.ID
	c.pendingErr = ""
	if wire.Error != nil {
		c.pendingErr = *wire.Error
	}
	if wire.Result != nil {
		c.pendingResult = append(c.pendingResult[:0], (*wire.Result)...) // copy
	} else {
		c.pendingResult = c.pendingResult[:0]
	}

	// Fill rpc.Response header so net/rpc can match the call.
	resp.ServiceMethod = ""   // not used by client matching
	resp.Seq = c.pendingSeq   // IMPORTANT
	resp.Error = c.pendingErr // net/rpc will surface this later

	slog.Debug("[client-codec] Finished ReadResponseHeader")

	return nil
}

func (c *unixJSONClientCodec) ReadResponseBody(body any) error {
	if c.pendingErr != "" {
		// Let net/rpc wrap it as error from Call()
		return nil
	}
	if body == nil || len(c.pendingResult) == 0 {
		return nil
	}
	return json.Unmarshal(c.pendingResult, body)
}

func (c *unixJSONClientCodec) Close() error { return c.uc.Close() }

// Helper your client uses after Call():
func (c *unixJSONClientCodec) takeLastFDs() []int {
	f := c.pendingFDs
	c.pendingFDs = nil
	return f
}
