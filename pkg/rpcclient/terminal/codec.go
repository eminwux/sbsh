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

package terminal

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/rpc"
	"sync"

	"github.com/eminwux/sbsh/internal/shared"
	"golang.org/x/sys/unix"
)

type unixJSONClientCodec struct {
	logger *slog.Logger
	uc     *shared.LoggingConnUnix
	encMu  sync.Mutex
	enc    *json.Encoder

	// stash from last header read:
	pendingMu     sync.RWMutex // protects pending fields
	pendingResult []byte
	pendingFDs    []int
	pendingSeq    uint64
	pendingErr    string
}

func newUnixJSONClientCodec(u *net.UnixConn, logger *slog.Logger) *unixJSONClientCodec {
	luc := &shared.LoggingConnUnix{
		UnixConn:    u,
		Logger:      logger,
		PrefixWrite: "client->server",
		PrefixRead:  "server->client",
	}
	return &unixJSONClientCodec{
		logger: logger,
		uc:     luc,
		enc:    json.NewEncoder(luc), // writes go through the logger
	}
}

func (c *unixJSONClientCodec) WriteRequest(req *rpc.Request, body any) error {
	c.logger.Debug("WriteRequest: acquiring encoder lock", "seq", req.Seq, "method", req.ServiceMethod)
	c.encMu.Lock()
	defer c.encMu.Unlock()

	wire := struct {
		ID     uint64         `json:"id"`
		Method string         `json:"method"`
		Params [1]interface{} `json:"params"`
	}{ID: req.Seq, Method: req.ServiceMethod, Params: [1]any{body}}

	// Encode to buffer (so we can inspect it before sending)
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&wire); err != nil {
		c.logger.Error("WriteRequest: failed to encode to buffer", "error", err)
		return err
	}

	c.logger.Debug("WriteRequest: encoding to wire", "seq", req.Seq, "method", req.ServiceMethod)
	err := c.enc.Encode(&wire)
	if err != nil {
		c.logger.Error("WriteRequest: failed to encode to wire", "error", err)
		return err
	}
	c.logger.Info("WriteRequest: request sent", "seq", req.Seq, "method", req.ServiceMethod)
	return nil
}

func (c *unixJSONClientCodec) ReadResponseHeader(resp *rpc.Response) error {
	c.logger.Debug("ReadResponseHeader: starting")

	// Block using netpoller-aware ReadMsgUnix.
	//nolint:mnd // 64KB buffer
	buf := make([]byte, 64<<10)
	//nolint:mnd // 256 bytes for FDs
	oob := make([]byte, 256)

	c.logger.Debug("ReadResponseHeader: calling ReadMsgUnix")
	n, oobn, _, errRead := c.uc.ReadMsgUnix(buf, oob)
	if errRead != nil {
		c.logger.Error("ReadResponseHeader: ReadMsgUnix failed", "error", errRead)
		return errRead
	}

	c.logger.Debug("ReadResponseHeader: parsing FDs", "oobn", oobn)
	c.pendingMu.Lock()
	c.pendingFDs = c.pendingFDs[:0]
	if oobn > 0 {
		if cmsgs, _ := unix.ParseSocketControlMessage(oob[:oobn]); len(cmsgs) > 0 {
			for _, m := range cmsgs {
				if fds, _ := unix.ParseUnixRights(&m); len(fds) > 0 {
					c.logger.Info("ReadResponseHeader: received FDs", "count", len(fds))
					c.pendingFDs = append(c.pendingFDs, fds...)
				}
			}
		}
	}

	c.logger.Debug("ReadResponseHeader: unmarshalling JSON")
	var wire struct {
		ID     uint64           `json:"id"`
		Result *json.RawMessage `json:"result"`
		Error  *string          `json:"error"`
	}
	if errUnmarshal := json.Unmarshal(buf[:n], &wire); errUnmarshal != nil {
		c.pendingMu.Unlock()
		c.logger.Error("ReadResponseHeader: JSON unmarshal failed", "error", errUnmarshal)
		return errUnmarshal
	}
	c.logger.Debug("ReadResponseHeader: JSON unmarshalled", "id", wire.ID)

	c.pendingSeq = wire.ID
	c.pendingErr = ""
	if wire.Error != nil {
		c.logger.Warn("ReadResponseHeader: error in response", "error", *wire.Error)
		c.pendingErr = *wire.Error
	}
	if wire.Result != nil {
		c.logger.Debug("ReadResponseHeader: result present", "result_len", len(*wire.Result))
		c.pendingResult = append(c.pendingResult[:0], (*wire.Result)...) // copy
	} else {
		c.logger.Debug("ReadResponseHeader: no result present")
		c.pendingResult = c.pendingResult[:0]
	}
	c.pendingMu.Unlock()

	// Fill rpc.Response header so net/rpc can match the call.
	c.pendingMu.RLock()
	pendingSeq := c.pendingSeq
	pendingErr := c.pendingErr
	c.pendingMu.RUnlock()
	resp.ServiceMethod = "" // not used by client matching
	resp.Seq = pendingSeq   // IMPORTANT
	resp.Error = pendingErr // net/rpc will surface this later

	c.logger.Info("ReadResponseHeader: finished", "seq", resp.Seq, "error", resp.Error)
	return nil
}

func (c *unixJSONClientCodec) ReadResponseBody(body any) error {
	c.pendingMu.Lock()
	pendingErr := c.pendingErr
	pendingResult := c.pendingResult
	c.pendingMu.Unlock()

	if pendingErr != "" {
		c.logger.Warn("ReadResponseBody: skipping due to error", "error", pendingErr)
		// Let net/rpc wrap it as error from Call()
		return nil
	}
	if body == nil || len(pendingResult) == 0 {
		c.logger.Debug(
			"ReadResponseBody: nothing to unmarshal",
			"body_nil",
			body == nil,
			"result_len",
			len(pendingResult),
		)
		return nil
	}
	err := json.Unmarshal(pendingResult, body)
	if err != nil {
		c.logger.Error("ReadResponseBody: unmarshal failed", "error", err)
		return err
	}
	c.logger.Info("ReadResponseBody: body unmarshalled successfully")
	return nil
}

func (c *unixJSONClientCodec) Close() error {
	c.logger.Debug("Close: closing unixJSONClientCodec")
	return c.uc.Close()
}

// Helper your client uses after Call():.
func (c *unixJSONClientCodec) takeLastFDs() []int {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.logger.Debug("takeLastFDs: returning and clearing FDs", "count", len(c.pendingFDs))
	f := c.pendingFDs
	c.pendingFDs = nil
	return f
}
