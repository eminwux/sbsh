package supervisorrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sbsh/pkg/api"
	"syscall"
	"time"

	"github.com/creack/pty"
)

func (s *SupervisorRunnerExec) attachAndForwardResize() error {

	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, err := pty.Getsize(os.Stdin); err == nil {

		ctx, cancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
		defer cancel()

		if err := s.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: int(cols), Rows: int(rows)}); err != nil {
			return fmt.Errorf("status failed: %w", err)
		}

	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ch:
				// slog.Debug("[supervisor] window change\r\n")
				// Query current terminal size again on every WINCH
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil {
					// harmless: keep going; terminal may be detached briefly
					continue
				}
				ctx, cancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
				defer cancel()
				if err := s.sessionClient.Resize(ctx, &api.ResizeArgs{Cols: int(cols), Rows: int(rows)}); err != nil {
					// Don't kill the process on resize failure; just log
					slog.Debug(fmt.Sprintf("resize RPC failed: %v\r\n", err))
				}
			}
		}
	}()
	return nil
}
