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

package dualcopier

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"golang.org/x/sync/errgroup"
)

type Copier struct {
	ctx      context.Context
	logger   *slog.Logger
	errGroup *errgroup.Group
}

func NewCopier(ctx context.Context, logger *slog.Logger) *Copier {
	errGroup, newCtx := errgroup.WithContext(ctx)

	return &Copier{
		ctx:      newCtx,
		logger:   logger,
		errGroup: errGroup,
	}
}

func (sr *Copier) readWriteBytes(r io.Reader, w io.Writer) error {
	//nolint:mnd // 32 KiB buffer
	buf := make([]byte, 32*1024)
	var total int64
	n, rerr := r.Read(buf)
	sr.logger.DebugContext(sr.ctx, "stdin->socket post-read", "n", n)

	if n > 0 {
		written := 0
		for written < n {
			sr.logger.DebugContext(sr.ctx, "stdin->socket pre-write")
			m, werr := w.Write(buf[written:n])
			sr.logger.DebugContext(sr.ctx, "stdin->socket post-write", "m", m)
			if werr != nil {
				return fmt.Errorf("could not write stdin->socket: %w", werr)
			}
			written += m
			total += int64(m)
		}
	}

	if rerr != nil {
		sr.logger.ErrorContext(sr.ctx, "stdin->socket read error", "error", rerr)
		return fmt.Errorf("could not read stdin->socket: %w", rerr)
	}

	return nil
}

func (sr *Copier) CopierManager(uc *net.UnixConn, finish func()) {
	errGroup := make(chan error, 1)
	go func() {
		errGroup <- sr.errGroup.Wait()
	}()

	select {
	case <-sr.ctx.Done():
		sr.logger.InfoContext(sr.ctx, "connManager: context cancelled or error received, beginning shutdown")
		// ACT IMMEDIATELY: close/unblock I/O, log, metrics, etc.
		//nolint:nestif // thorough debug
		if uc != nil {
			if err := uc.SetReadDeadline(time.Now()); err != nil {
				sr.logger.WarnContext(sr.ctx, "connManager: failed to set read deadline", "error", err)
			} else {
				sr.logger.DebugContext(sr.ctx, "connManager: set read deadline to unblock readers")
			}
			if err := uc.SetWriteDeadline(time.Now()); err != nil {
				sr.logger.WarnContext(sr.ctx, "connManager: failed to set write deadline", "error", err)
			} else {
				sr.logger.DebugContext(sr.ctx, "connManager: set write deadline to unblock writers")
			}
		} else {
			sr.logger.WarnContext(sr.ctx, "connManager: UnixConn is nil, cannot set deadlines")
		}
		sr.logger.InfoContext(sr.ctx, "connManager: context done, shutting down connection manager")

		finish()

	case err := <-errGroup:
		e := fmt.Errorf("wait on routines error group returned: %w", err)
		sr.logger.ErrorContext(sr.ctx, "read/write routines finished", "error", e)
	}
}

func (sr *Copier) runReadWriter(r io.Reader, w io.Writer, ready chan struct{}, errFunc func()) error {
	close(ready)
	for {
		select {
		case <-sr.ctx.Done():
			return sr.ctx.Err()
		default:
			sr.logger.DebugContext(
				sr.ctx,
				"pre-read",
				"reader",
				fmt.Sprintf("%T", r),
				"writer",
				fmt.Sprintf("%T", w),
			)
			err := sr.readWriteBytes(r, w)
			if err != nil {
				sr.logger.ErrorContext(
					sr.ctx,
					"read/write error",
					"reader",
					fmt.Sprintf("%T", r),
					"writer",
					fmt.Sprintf("%T", w),
					"error",
					err,
				)
				errFunc()
				return err
			}
		}
	}
}

func (sr *Copier) RunCopier(r io.Reader, w io.Writer, ready chan struct{}, errFunc func()) {
	sr.logger.DebugContext(
		sr.ctx,
		"RunCopier: starting copier goroutine",
		"reader", fmt.Sprintf("%T", r),
		"writer", fmt.Sprintf("%T", w),
	)
	sr.errGroup.Go(func() error {
		sr.logger.DebugContext(
			sr.ctx,
			"RunCopier: copier goroutine running",
			"reader", fmt.Sprintf("%T", r),
			"writer", fmt.Sprintf("%T", w),
		)
		return sr.runReadWriter(r, w, ready, errFunc)
	})
}
