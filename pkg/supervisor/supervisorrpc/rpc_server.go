package supervisorrpc

import (
	"sbsh/pkg/api"
)

type SupervisorControllerRPC struct {
	Core api.SupervisorController // the real server-side controller
}

// Optional: usually you don’t expose Run over RPC because it blocks.
func (s *SupervisorControllerRPC) WaitReady(_ *api.Empty, _ *api.Empty) error {
	return s.Core.WaitReady()
}

func (s *SupervisorControllerRPC) Detach(_ *api.Empty, _ *api.Empty) error {
	return s.Core.Detach()
}

// TODO
// show current attach session
// attach to a different session
// detach from session and exit
