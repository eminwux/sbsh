package session

type SessionControllerRPC struct {
	Core SessionController
}

func (r *SessionControllerRPC) Status(_ struct{}, out *string) error {
	*out = r.Core.Status()
	return nil
}
