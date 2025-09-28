package supervisorrunner

import (
	"fmt"
	"log/slog"
	"sbsh/pkg/api"
	"time"
)

type SupervisorRunnerEvent struct {
	ID    api.ID
	Type  SupervisorRunnerEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

type SupervisorRunnerEventType int

const (
	EvError SupervisorRunnerEventType = iota // abnormal error
	EvCmdExited
)

// helper: non-blocking event send so the PTY reader never stalls
func trySendEvent(ch chan<- SupervisorRunnerEvent, ev SupervisorRunnerEvent) {
	slog.Debug(fmt.Sprintf("[supervisor] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano)))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
