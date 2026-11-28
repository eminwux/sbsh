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

// Package capture holds the writer side of sbsh's capture format: the
// asciicast line encoders and segment compression used by the rotating capture
// writer in internal/terminal/terminalrunner. Readers live in pkg/capture,
// which also documents the on-disk contract.
package capture

import (
	"encoding/json"
	"fmt"

	pcapture "github.com/eminwux/sbsh/pkg/capture"
)

// EncodeHeader marshals an asciicast v2 header line (newline-terminated).
// width/height record the terminal geometry in effect when the line is written
// (the launch size, or the latest resize); timestamp is the unix epoch
// (seconds) of the run's start.
func EncodeHeader(width, height int, timestamp int64) ([]byte, error) {
	b, err := json.Marshal(pcapture.Header{
		Version:   pcapture.AsciicastVersion,
		Width:     width,
		Height:    height,
		Timestamp: timestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal asciicast header: %w", err)
	}
	return append(b, '\n'), nil
}

// EncodeOutputEvent marshals one asciicast v2 output record line:
// `[<elapsed-seconds>, "o", <data>]\n`. data is wrapped in a JSON string, so
// arbitrary bytes are escaped (and non-UTF-8 bytes coerced by the encoder —
// raw is the byte-exact format; asciicast trades that for replay/portability).
func EncodeOutputEvent(elapsed float64, data []byte) ([]byte, error) {
	b, err := json.Marshal([]any{elapsed, pcapture.EventOutput, string(data)})
	if err != nil {
		return nil, fmt.Errorf("marshal asciicast event: %w", err)
	}
	return append(b, '\n'), nil
}

// EncodeResizeEvent marshals one asciicast v2 resize record line:
// `[<elapsed-seconds>, "r", "<cols>x<rows>"]\n`.
func EncodeResizeEvent(elapsed float64, cols, rows int) ([]byte, error) {
	b, err := json.Marshal([]any{elapsed, pcapture.EventResize, fmt.Sprintf("%dx%d", cols, rows)})
	if err != nil {
		return nil, fmt.Errorf("marshal asciicast resize event: %w", err)
	}
	return append(b, '\n'), nil
}
