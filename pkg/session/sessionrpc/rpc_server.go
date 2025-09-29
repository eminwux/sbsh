package sessionrpc

import (
	"sbsh/pkg/api"
)

type SessionControllerRPC struct {
	Core api.SessionController
}

func (r *SessionControllerRPC) Status(_ *api.Empty, out *api.SessionStatusMessage) error {
	*out = api.SessionStatusMessage{Message: r.Core.Status()}
	return nil
}
func (s *SessionControllerRPC) Resize(args api.ResizeArgs, _ *api.Empty) error {
	s.Core.Resize(args)
	return nil
}

func (s *SessionControllerRPC) Detach(_ *api.Empty, _ *api.Empty) error {
	return s.Core.Detach()
}

// TODO
// show session details, including attach status
// attach, redirects pipe output/input to socket
// dettach, redirects pipe output to log, input to null
// close session
// restart session
