package sessionrunner

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"sbsh/pkg/api"
	"sync"
	"time"
)

type SessionRunnerExec struct {
	ctx context.Context

	// immutable
	id   api.ID
	spec api.SessionSpec

	// runtime (owned by Session)
	cmd     *exec.Cmd
	pty     *os.File // master
	state   api.SessionState
	runPath string

	gates struct {
		StdinOpen bool
		OutputOn  bool
	}

	// observability
	bytesIn, bytesOut uint64
	lastRead          time.Time

	// signaling
	evCh chan<- SessionRunnerEvent // fan-out to controller (send-only from session)

	listenerIO   net.Listener
	listenerCtrl net.Listener

	socketIO     string
	socketCtrl   string
	metadataFile string

	clientsMu sync.RWMutex
	clients   map[int]*ioClient

	closeReqCh chan error
	closedCh   chan struct{}

	ptyPipes *ptyPipes
}

type ptyPipes struct {
	pipeInR   *os.File
	pipeInW   *os.File
	pipeOutR  *os.File
	pipeOutW  *os.File
	multiOutW io.Writer
}

type ioClient struct {
	id       int
	conn     net.Conn
	pipeInR  *os.File
	pipeInW  *os.File
	pipeOutR *os.File
	pipeOutW *os.File
}

const deleteSessionDir bool = false

var finishTermMgr chan struct{} = make(chan struct{}, 2)

func NewSessionRunnerExec(ctx context.Context, spec *api.SessionSpec) SessionRunner {
	return &SessionRunnerExec{
		id:   spec.ID,
		spec: *spec,
		ctx:  ctx,

		// runtime (initialized but inactive)
		cmd:     nil,
		pty:     nil,
		state:   api.SessBash, // default logical state before start
		runPath: spec.RunPath + "/sessions",

		gates: struct {
			StdinOpen bool
			OutputOn  bool
		}{
			StdinOpen: true, // allow stdin by default once started
			OutputOn:  true, // render PTY output by default
		},

		// observability (zeroed; will be updated when running)
		bytesIn:  0,
		bytesOut: 0,

		// signaling (set in Start)
		evCh: nil, // assigned in Start(...)

		closeReqCh: make(chan error),
		closedCh:   make(chan struct{}),
		ptyPipes:   &ptyPipes{},
	}
}

func (sr *SessionRunnerExec) ID() api.ID {
	return sr.id
}
