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

package terminalrunner

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// WritePTY pushes a single bulk write to the PTY master. Unlike the
// byte-at-a-time path used for shell-init stuffing, this path sends the
// caller's buffer verbatim: user-driven input must not be reshaped.
func (sr *Exec) WritePTY(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	sr.obsMu.RLock()
	stdinOpen := sr.gates.StdinOpen
	sr.obsMu.RUnlock()
	if !stdinOpen {
		return errdefs.ErrTerminalStdinClosed
	}

	if sr.ptmx == nil {
		return errdefs.ErrTerminalStdinClosed
	}

	if _, err := sr.ptmx.Write(data); err != nil {
		return fmt.Errorf("write to pty: %w", err)
	}

	sr.obsMu.Lock()
	sr.bytesIn += uint64(len(data))
	sr.obsMu.Unlock()
	return nil
}

// Subscribe registers a new output-only consumer on the PTY fan-out.
// It allocates a socketpair, returns the client-side FD via SCM_RIGHTS
// on the RPC response, and feeds PTY output into a bounded ring whose
// drain goroutine writes to the server-side conn. If the subscriber
// falls behind it is disconnected with a lagged-notice sentinel rather
// than stalling the fan-out.
func (sr *Exec) Subscribe(req *api.SubscribeRequest, response *api.ResponseWithFD) error {
	clientID := api.ID("")
	if req != nil {
		clientID = req.ClientID
	}
	sr.logger.Debug("Subscribe: creating socketpair", "client", clientID)

	sv, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|sockCloexec, 0)
	if err != nil {
		sr.logger.Error("Subscribe: Socketpair failed", "client", clientID, "error", err)
		return fmt.Errorf("Subscribe: Socketpair: %w", err)
	}
	srvFD := sv[0]
	cliFD := sv[1]

	f := os.NewFile(uintptr(srvFD), "terminal-subscribe")
	conn, err := net.FileConn(f)
	if err != nil {
		_ = f.Close()
		_ = unix.Close(cliFD)
		return fmt.Errorf("Subscribe: FileConn: %w", err)
	}
	_ = f.Close()

	// We only ever write to subscribers; close the read half so a stuck
	// subscriber can't fill the kernel buffer in the reverse direction.
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseRead()
	}

	sr.ptyPipesMu.RLock()
	multiOutW := sr.ptyPipes.multiOutW
	sr.ptyPipesMu.RUnlock()
	if multiOutW == nil {
		_ = conn.Close()
		_ = unix.Close(cliFD)
		return fmt.Errorf("Subscribe: terminal not running")
	}

	// Early shutdown check: avoid registry mutation when Close has already
	// flipped the context. Cheaper than the late check below and rejects
	// late RPCs that arrive on a per-conn handler still draining after the
	// accept loop's ctx-cancel exit. See #226.
	if sr.ctx.Err() != nil {
		_ = conn.Close()
		_ = unix.Close(cliFD)
		return errTerminalClosing
	}

	var sub *subscriberWriter
	var detachOnce sync.Once
	detach := func() {
		detachOnce.Do(func() {
			multiOutW.Remove(sub)
			sr.removeSubscriber(sub)
			sr.logger.Info("Subscribe: subscriber detached", "client", clientID)
		})
	}

	sub = newSubscriberWriter(conn, defaultSubscriberBufferBytes, detach, sr.logger)
	sr.addSubscriber(sub)
	multiOutW.Add(sub)
	go sub.Run()

	// Late shutdown check: Close's closeAllSubscribers may have already
	// snapshotted sr.subscribers before our addSubscriber landed (Close
	// runs ctxCancel before snapshotting), leaving the drain parked on
	// cond.Wait() with no writer to signal it. Re-check ctx after the
	// goroutine is live and, if shutdown observed, Close the subscriber
	// ourselves — drain wakes, runs onDetach (multiOutW.Remove +
	// removeSubscriber) and closes conn via its deferred close. Both
	// detach() and sub.Close() are idempotent, so concurring with Close's
	// own iteration is harmless. See #226.
	if sr.ctx.Err() != nil {
		_ = sub.Close()
		_ = unix.Close(cliFD)
		return errTerminalClosing
	}

	payload := struct {
		OK bool `json:"ok"`
	}{OK: true}

	response.JSON = payload
	response.FDs = []int{cliFD}
	sr.logger.Info("Subscribe: subscriber registered", "client", clientID)
	return nil
}

func (sr *Exec) addSubscriber(s *subscriberWriter) {
	sr.subsMu.Lock()
	sr.subscribers[s] = struct{}{}
	sr.subsMu.Unlock()
}

func (sr *Exec) removeSubscriber(s *subscriberWriter) {
	sr.subsMu.Lock()
	delete(sr.subscribers, s)
	sr.subsMu.Unlock()
}

func (sr *Exec) closeAllSubscribers() {
	sr.subsMu.Lock()
	subs := make([]*subscriberWriter, 0, len(sr.subscribers))
	for s := range sr.subscribers {
		subs = append(subs, s)
	}
	sr.subsMu.Unlock()
	for _, s := range subs {
		_ = s.Close()
	}
}
