package session

import (
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
)

type FakeSessionController struct {
	Exit chan error

	AddedSpec *api.SessionSpec

	RunFunc       func(spec *api.SessionSpec) error
	WaitReadyFunc func() error
	WaitCloseFunc func() error
	StatusFunc    func() string
	CloseFunc     func(reason error) error
	ResizeFunc    func()
	DetachFunc    func() error
}

func (f *FakeSessionController) Run(spec *api.SessionSpec) error {
	f.AddedSpec = spec
	if f.RunFunc != nil {
		return f.RunFunc(spec)
	}
	return errdefs.ErrFuncNotSet
}
func (f *FakeSessionController) WaitReady() error {
	if f.WaitReadyFunc != nil {
		return f.WaitReadyFunc()
	}
	return errdefs.ErrFuncNotSet
}
func (f *FakeSessionController) WaitClose() error {
	if f.WaitCloseFunc != nil {
		return f.WaitCloseFunc()
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) Status() string {
	if f.StatusFunc != nil {
		return f.StatusFunc()
	}
	return ""
}

func (f *FakeSessionController) Close(reason error) error {
	if f.CloseFunc != nil {
		return f.CloseFunc(reason)
	}
	return errdefs.ErrFuncNotSet
}

func (f *FakeSessionController) Resize(args api.ResizeArgs) {
	if f.ResizeFunc != nil {
		f.ResizeFunc()
	}
}

func (f *FakeSessionController) Detach() error {
	if f.DetachFunc() != nil {
		return f.DetachFunc()
	}
	return errdefs.ErrFuncNotSet
}
