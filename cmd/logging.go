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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type ReformatHandler struct {
	inner slog.Handler
}

func (h *ReformatHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.inner.Enabled(ctx, lvl)
}

func (h *ReformatHandler) Handle(ctx context.Context, r slog.Record) error {
	ts := r.Time.Format("2006-01-02T15:04:05Z07:00")
	level := strings.ToUpper(r.Level.String())
	msg := fmt.Sprintf("%q", r.Message) // quoted message

	attrs := ""
	r.Attrs(func(a slog.Attr) bool {
		attrs += fmt.Sprintf(" %s=%v", a.Key, a.Value)
		return true
	})

	fmt.Fprintf(os.Stdout, "%s %s %s%s\n", ts, level, msg, attrs)
	return nil
}

func (h *ReformatHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ReformatHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ReformatHandler) WithGroup(name string) slog.Handler {
	return &ReformatHandler{inner: h.inner.WithGroup(name)}
}
