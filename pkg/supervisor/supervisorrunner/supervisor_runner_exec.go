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
	id                   api.ID
	spec                 api.SupervisorSpec
	runPath              string
	session              *sessionstore.SupervisedSession
	ctx                  context.Context
	uiMode               UIMode
	events               chan<- SupervisorRunnerEvent
	lastTermState        *term.State
	Mgr                  *sessionstore.SessionStoreExec
	supervisorSocketCtrl string
	lnCtrl               net.Listener
	sessionClient        session.Client
}

type UIMode int

const (
	UIBash UIMode = iota
	UISupervisor
	UIExitShell // Saved lastState restore
)

func NewSupervisorRunnerExec(ctx context.Context, spec *api.SupervisorSpec) SupervisorRunner {
	return &SupervisorRunnerExec{
		ctx:     ctx,
		id:      spec.ID,
		spec:    *spec,
		runPath: spec.RunPath + "/supervisors",
	}
}
