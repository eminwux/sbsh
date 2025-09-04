package supervisor

import (
	"context"
	"sbsh/pkg/api"
)

type RPCController struct {
	Core SupervisorController // the real server-side controller
}

// Optional: usually you don’t expose Run over RPC because it blocks.
func (s *RPCController) WaitReady(_ *api.Empty, _ *api.Empty) error {
	return s.Core.WaitReady(context.Background())
}

func (s *RPCController) AddSession(args *api.AddSessionArgs, _ *api.Empty) error {
	spec := args.Spec
	s.Core.AddSession(&spec)
	return nil
}

func (s *RPCController) SetCurrentSession(args *api.SessionIDArg, _ *api.Empty) error {
	return s.Core.SetCurrentSession(args.ID)
}

func (s *RPCController) StartSession(args *api.SessionIDArg, _ *api.Empty) error {
	return s.Core.StartSession(args.ID)
}
