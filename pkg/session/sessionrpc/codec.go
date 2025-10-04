package sessionrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/rpc"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sync"

	"golang.org/x/sys/unix"
)

// unixJSONServerCodec: JSON-RPC over *UnixConn with optional FD passing via SCM_RIGHTS.
type unixJSONServerCodec struct {
	uc *common.LoggingConnUnix

	dec *json.Decoder

	mu sync.Mutex // serialize writes (including sendmsg)

	// seq bookkeeping
	seqMu       sync.Mutex
	nextSeq     uint64
	pendingID   map[uint64]any             // serverSeq -> wire JSON id (number or string)
	paramsBySeq map[uint64]json.RawMessage // serverSeq -> raw params array
}

func NewUnixJSONServerCodec(uc *net.UnixConn) rpc.ServerCodec {
	luc := &common.LoggingConnUnix{
		UnixConn:    uc,
		PrefixWrite: "server->client",
		PrefixRead:  "client->server",
	}
	return &unixJSONServerCodec{
		uc:          luc,
		dec:         json.NewDecoder(luc),
		pendingID:   make(map[uint64]any),
		paramsBySeq: make(map[uint64]json.RawMessage),
	}
}

// ReadRequestHeader reads one JSON-RPC request and stashes wire id + params.
// We assign our own server sequence (monotonic) for net/rpc routing.
func (c *unixJSONServerCodec) ReadRequestHeader(r *rpc.Request) error {
	var req struct {
		ID     any             `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := c.dec.Decode(&req); err != nil {
		return err
	}

	c.seqMu.Lock()
	c.nextSeq++
	seq := c.nextSeq
	c.pendingID[seq] = req.ID
	if len(req.Params) > 0 {
		c.paramsBySeq[seq] = req.Params
	}
	c.seqMu.Unlock()

	r.ServiceMethod = req.Method
	r.Seq = seq
	return nil
}

// ReadRequestBody unmarshals the first element of the params array into body (like std jsonrpc).
func (c *unixJSONServerCodec) ReadRequestBody(body interface{}) error {
	if body == nil {
		return nil
	}
	// We don't get seq here, so we rely on json.Decoder’s sequencing; net/rpc calls
	// ReadRequestBody immediately after ReadRequestHeader on the same goroutine,
	// so the most recent seq is r.Seq. To keep this simple, we pop the smallest seq
	// that still has params (the one we just set).
	c.seqMu.Lock()
	var chosenSeq uint64
	for s := range c.paramsBySeq {
		if chosenSeq == 0 || s < chosenSeq {
			chosenSeq = s
		}
	}
	params := c.paramsBySeq[chosenSeq]
	delete(c.paramsBySeq, chosenSeq)
	c.seqMu.Unlock()

	if len(params) == 0 || string(params) == "null" {
		return nil
	}

	// Params are an array: [arg]
	var arr []json.RawMessage
	if err := json.Unmarshal(params, &arr); err != nil {
		return err
	}
	if len(arr) == 0 {
		return nil
	}
	return json.Unmarshal(arr[0], body)
}

func (c *unixJSONServerCodec) Close() error {
	return c.uc.Close()
}

func (c *unixJSONServerCodec) WriteResponse(resp *rpc.Response, body interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Debug("[rpc] WriteResponse",
		"service", resp.ServiceMethod,
		"seq", resp.Seq,
		"error", resp.Error,
		"body_type", fmt.Sprintf("%T", body),
	)

	// Resolve the original wire JSON id for this server seq.
	c.seqMu.Lock()
	wireID, hasID := c.pendingID[resp.Seq]
	delete(c.pendingID, resp.Seq)
	c.seqMu.Unlock()
	if !hasID {
		// Fallback (shouldn't happen): use server seq.
		wireID = resp.Seq
	}

	// Pretty-log body (optional)
	if body != nil {
		if b, err := json.MarshalIndent(body, "", "  "); err == nil {
			slog.Debug("[rpc] WriteResponse body content", "json", string(b))
		} else {
			slog.Debug("[rpc] WriteResponse body marshal failed", "err", err)
		}
	}

	// FD-passing path
	if r, ok := body.(*api.ResponseWithFD); ok && len(r.FDs) > 0 {
		slog.Debug("[session] Attach codec response", "json_payload", r.JSON, "fds", r.FDs)

		wire := struct {
			ID     any         `json:"id"`
			Result interface{} `json:"result"`
			Error  interface{} `json:"error"`
		}{ID: wireID, Result: r.JSON, Error: nil}

		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&wire); err != nil {
			return err
		}

		rights := unix.UnixRights(r.FDs...)
		if _, _, err := c.uc.WriteMsgUnix(buf.Bytes(), rights, nil); err != nil {
			return err
		}
		return nil
	}

	// Normal (no FD) response: write JSON with the correct wire id.
	wire := struct {
		ID     any         `json:"id"`
		Result interface{} `json:"result"`
		Error  interface{} `json:"error"`
	}{
		ID:     wireID,
		Result: body,
		Error:  nil,
	}
	// If resp.Error is non-empty, jsonrpc expects "result": null and "error": <string/obj>.
	if resp.Error != "" {
		wire.Result = nil
		wire.Error = resp.Error
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&wire); err != nil {
		return err
	}
	_, err := c.uc.Write(buf.Bytes())
	return err
}
