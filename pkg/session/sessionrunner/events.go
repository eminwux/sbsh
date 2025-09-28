package sessionrunner

import (
	"fmt"
	"log/slog"
	"sbsh/pkg/api"
	"time"
)

type SessionRunnerEventType int

type SessionRunnerEvent struct {
	ID    api.ID
	Type  SessionRunnerEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

const (
	EvError SessionRunnerEventType = iota // abnormal error
	EvCmdExited
)

// helper: non-blocking event send so the PTY reader never stalls
func trySendEvent(ch chan<- SessionRunnerEvent, ev SessionRunnerEvent) {
	slog.Debug(fmt.Sprintf("[session] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano)))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
