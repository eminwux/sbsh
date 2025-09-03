package session

type sessionRPC struct{ S *Session }

// func (r *sessionRPC) Info(_ struct{}, out *api.SessionInfo) error {
// 	*out = r.S.Info()
// 	return nil
// }

// func (r *sessionRPC) Resize(args api.ResizeArgs, _ *api.Empty) error {
// 	return r.S.ResizeTo(args.Cols, args.Rows)
// }
// func (r *sessionRPC) Shutdown(_ struct{}, _ *api.Empty) error {
// 	go r.S.Close()
// 	return nil
// }
