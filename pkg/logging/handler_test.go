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

package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/pkg/logging"
)

// TestReformatHandler_WithAttrsPropagatesWriter guards against the
// derived-handler nil-Writer regression: WithAttrs must thread the
// Writer through so logger.With(...) does not panic in Handle.
func TestReformatHandler_WithAttrsPropagatesWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	h := &logging.ReformatHandler{
		Inner:  slog.NewTextHandler(buf, nil),
		Writer: buf,
	}
	logger := slog.New(h).With("attached", "yes")

	logger.Info("hello")

	got := buf.String()
	if !strings.Contains(got, "INFO") || !strings.Contains(got, `"hello"`) {
		t.Errorf("derived logger output = %q, missing ReformatHandler markers", got)
	}
}

// TestReformatHandler_WithGroupPropagatesWriter mirrors the WithAttrs
// guard for WithGroup.
func TestReformatHandler_WithGroupPropagatesWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	h := &logging.ReformatHandler{
		Inner:  slog.NewTextHandler(buf, nil),
		Writer: buf,
	}
	logger := slog.New(h).WithGroup("scope")

	logger.Info("hello")

	got := buf.String()
	if !strings.Contains(got, "INFO") || !strings.Contains(got, `"hello"`) {
		t.Errorf("grouped logger output = %q, missing ReformatHandler markers", got)
	}
}
