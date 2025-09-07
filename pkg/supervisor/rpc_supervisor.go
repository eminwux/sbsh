package supervisor

import (
	"context"
	"sbsh/pkg/api"
)

type SupervisorControllerRPC struct {
	Core SupervisorController // the real server-side controller
}

// Optional: usually you don’t expose Run over RPC because it blocks.
func (s *SupervisorControllerRPC) WaitReady(_ *api.Empty, _ *api.Empty) error {
	return s.Core.WaitReady(context.Background())
}

func (s *SupervisorControllerRPC) AddSession(args *api.AddSessionArgs, _ *api.Empty) error {
	spec := args.Spec
	s.Core.AddSession(&spec)
	return nil
}

func (s *SupervisorControllerRPC) SetCurrentSession(args *api.SessionIDArg, _ *api.Empty) error {
	return s.Core.SetCurrentSession(args.ID)
}

func (s *SupervisorControllerRPC) StartSession(args *api.SessionIDArg, _ *api.Empty) error {
	return s.Core.StartSession(args.ID)
}
