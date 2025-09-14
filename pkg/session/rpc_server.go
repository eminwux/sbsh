package session

import (
	"sbsh/pkg/api"
)

type SessionControllerRPC struct {
	Core SessionController
}

func (r *SessionControllerRPC) Status(_ *api.Empty, out *api.SessionStatus) error {
	*out = api.SessionStatus{Message: r.Core.Status()}
	return nil
}
func (s *SessionControllerRPC) Resize(args api.ResizeArgs, _ *api.Empty) error {
	s.Core.Resize(args)
	return nil
}
