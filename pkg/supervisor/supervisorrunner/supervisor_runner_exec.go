package supervisorrunner

import (
	"context"
	"net"
	"sbsh/pkg/api"
	"sbsh/pkg/rpcclient/session"
	"sbsh/pkg/supervisor/sessionstore"

	"golang.org/x/term"
)

type SupervisorRunnerExec struct {
	id   api.ID
	spec api.SupervisorSpec

	ctx       context.Context
	ctxCancel context.CancelFunc

	runPath              string
	supervisorSocketCtrl string

	uiMode        UIMode
	lastTermState *term.State

	events  chan<- SupervisorRunnerEvent
	session *sessionstore.SupervisedSession
	Mgr     *sessionstore.SessionStoreExec

	lnCtrl        net.Listener
	sessionClient session.Client
}

type UIMode int

const (
	UIBash UIMode = iota
	UISupervisor
	UIExitShell // Saved lastState restore
)

func NewSupervisorRunnerExec(ctx context.Context, spec *api.SupervisorSpec) SupervisorRunner {
	newCtx, cancel := context.WithCancel(ctx)

	return &SupervisorRunnerExec{
		id:   spec.ID,
		spec: *spec,

		ctx:       newCtx,
		ctxCancel: cancel,

		runPath: spec.RunPath + "/supervisors",
	}
}
