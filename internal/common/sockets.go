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

package common

import (
	"fmt"
	"log/slog"
	"net"
	"strings"

	"golang.org/x/sys/unix"
)

func LogBytes(prefix string, data []byte, logger *slog.Logger) {
	logHEX(prefix, data, logger)
	logASCII(prefix, data, logger)
}

// ASCII only (single-line attr, safe in any handler).
func logASCII(prefix string, data []byte, logger *slog.Logger) {
	if len(data) == 0 {
		logger.Debug(prefix+" (ascii)", "len", 0, "data", "(empty)")
		return
	}
	var b strings.Builder
	for _, c := range data {
		if c >= 32 && c < 127 {
			b.WriteByte(c)
		} else {
			b.WriteByte('.')
		}
	}
	logger.Debug(prefix+" (ascii)", "len", len(data), "data", b.String())
}

// HEX only (multiline message; no ASCII pane).
func logHEX(prefix string, data []byte, logger *slog.Logger) {
	if len(data) == 0 {
		logger.Debug(prefix + " (hex)")
		return
	}
	var b strings.Builder
	for off := 0; off < len(data); off += 16 {
		length := 16
		end := off + length
		if end > len(data) {
			end = len(data)
		}
		line := data[off:end]

		// hex columns fixed width (16 bytes → 47 chars: "HH " * 16 minus last space)
		var hexCols strings.Builder
		for i := range 16 {
			if i < len(line) {
				fmt.Fprintf(&hexCols, "%02X", line[i])
			} else {
				hexCols.WriteString("  ")
			}
			maxHex := 15
			if i != maxHex {
				hexCols.WriteByte(' ')
			}
		}

		// only offset + hex (no ASCII)
		fmt.Fprintf(&b, "%04X  %s\n", off, hexCols.String())
	}

	// Put the dump in the *message* so newlines render (text handler).
	// Most JSON handlers escape newlines; if you use JSON logs, consider per-line logs instead.
	logger.Debug(prefix+" (hex)\n"+b.String(), "len", len(data))
}

type LoggingConn struct {
	net.Conn
	*slog.Logger

	PrefixWrite string
	PrefixRead  string
}

type LoggingConnUnix struct {
	*net.UnixConn
	*slog.Logger

	PrefixWrite string
	PrefixRead  string
}

func (l *LoggingConn) Read(p []byte) (int, error) {
	n, err := l.Conn.Read(p)
	if n > 0 {
		LogBytes(l.PrefixRead+" (recv)", p[:n], l.Logger)
	}
	return n, err
}

func (l *LoggingConn) Write(p []byte) (int, error) {
	LogBytes(l.PrefixWrite+" (send)", p, l.Logger)
	return l.Conn.Write(p)
}

func (l *LoggingConnUnix) Read(p []byte) (int, error) {
	n, err := l.UnixConn.Read(p)
	if n > 0 {
		LogBytes(l.PrefixRead+" (recv)", p[:n], l.Logger)
	}
	return n, err
}

func (l *LoggingConnUnix) Write(p []byte) (int, error) {
	LogBytes(l.PrefixWrite+" (send)", p, l.Logger)
	return l.UnixConn.Write(p)
}

func (l *LoggingConnUnix) WriteMsgUnix(p, oob []byte, addr *net.UnixAddr) (int, int, error) {
	// payload
	LogBytes(l.PrefixWrite+" (send)", p, l.Logger)

	// OOB/FDs
	if len(oob) > 0 {
		if cmsgs, err := unix.ParseSocketControlMessage(oob); err == nil {
			var fds []int
			for _, m := range cmsgs {
				if fs, _ := unix.ParseUnixRights(&m); len(fs) > 0 {
					fds = append(fds, fs...)
				}
			}
			l.Logger.Debug(l.PrefixWrite+" (oob fds)", "fds", fds)
		} else {
			l.Logger.Error(l.PrefixWrite+" (oob parse error)", "err", err)
		}
	}
	return l.UnixConn.WriteMsgUnix(p, oob, addr)
}

func (l *LoggingConnUnix) ReadMsgUnix(p, oob []byte) (int, int, *net.UnixAddr, error) {
	n, oobn, _, addr, err := l.UnixConn.ReadMsgUnix(p, oob)
	if n > 0 {
		LogBytes(l.PrefixRead+" (recv)", p[:n], l.Logger)
	}
	if oobn > 0 {
		if cmsgs, err := unix.ParseSocketControlMessage(oob[:oobn]); err == nil {
			var fds []int
			for _, m := range cmsgs {
				if fs, _ := unix.ParseUnixRights(&m); len(fs) > 0 {
					fds = append(fds, fs...)
				}
			}
			l.Logger.Debug(l.PrefixRead+" (oob fds)", "fds", fds)
		} else {
			l.Logger.Error(l.PrefixRead+" (oob parse error)", "err", err)
		}
	}
	return n, oobn, addr, err
}
