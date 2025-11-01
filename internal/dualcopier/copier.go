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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/eminwux/sbsh/internal/filter"
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

//nolint:gocognit // detailed I/O logic
func (sr *Copier) readWriteBytes(r io.Reader, w io.Writer, f filter.Filter) error {
	//nolint:mnd // 32 KiB buffer
	buf := make([]byte, 32*1024)

	n, rerr := r.Read(buf)
	sr.logger.DebugContext(sr.ctx, "stdin->socket post-read", "n", n)

	//nolint:nestif // detailed I/O logic
	if n > 0 {
		written := 0
		for written < n {
			var (
				out           []byte
				nwrite, ncons int
				ferr          error
			)

			if f != nil {
				out, nwrite, ncons, ferr = f.Process(buf[written:], n-written)
				if ferr != nil {
					return ferr
				}
				// Safety: never allow zero-consume → infinite loop.
				if ncons == 0 && nwrite == 0 {
					// Fallback: consume 1 byte (forward it) to make progress.
					out = buf[written : written+1]
					nwrite, ncons = 1, 1
				}
			} else {
				// No filter: write the remainder in one go.
				out = buf[written:n]
				nwrite = len(out)
				ncons = n - written
			}

			// If the filter consumed bytes but wants nothing written, just advance.
			if nwrite == 0 {
				written += ncons
				continue
			}

			// Determine what to write: either the filter-provided slice, or the in-place window.
			var toWrite []byte
			if out != nil {
				if nwrite > len(out) {
					return fmt.Errorf("filter nwrite exceeds out length: %d > %d", nwrite, len(out))
				}
				toWrite = out[:nwrite]
			} else {
				// Write from the input buffer window.
				if written+nwrite > n {
					return fmt.Errorf("filter nwrite exceeds available input: %d > %d", nwrite, n-written)
				}
				toWrite = buf[written : written+nwrite]
			}

			// Perform the write (handle short writes).
			sr.logger.DebugContext(sr.ctx, "stdin->socket pre-write", "req", len(toWrite))
			total := 0
			for total < len(toWrite) {
				m, werr := w.Write(toWrite[total:])
				sr.logger.DebugContext(sr.ctx, "stdin->socket post-write", "m", m)
				if werr != nil {
					return fmt.Errorf("could not write stdin->socket: %w", werr)
				}
				if m == 0 {
					return errors.New("short write with no progress")
				}
				total += m
			}

			// Advance by what the filter says it consumed, not by what we wrote.
			written += ncons
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

func (sr *Copier) runReadWriter(
	r io.Reader,
	w io.Writer,
	ready chan struct{},
	errFunc func(),
	f filter.Filter,
) error {
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
			err := sr.readWriteBytes(r, w, f)
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

func (sr *Copier) RunCopier(r io.Reader, w io.Writer, ready chan struct{}, errFunc func(), f filter.Filter) {
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
		return sr.runReadWriter(r, w, ready, errFunc, f)
	})
}
