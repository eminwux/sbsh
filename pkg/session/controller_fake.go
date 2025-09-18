package session

import (
	"sbsh/pkg/api"
)

type FakeSessionController struct {
	Exit chan error

	AddedSpec *api.SessionSpec

	RunFunc       func(spec *api.SessionSpec)
	WaitReadyFunc func() error
	WaitCloseFunc func()
	StatusFunc    func() string
	CloseFunc     func() error
	ResizeFunc    func()
}

func (f *FakeSessionController) Run(spec *api.SessionSpec) {
	f.AddedSpec = spec
	if f.RunFunc != nil {
		f.RunFunc(spec)
	}
}
func (f *FakeSessionController) WaitReady() error {
	if f.WaitReadyFunc != nil {
		return f.WaitReadyFunc()
	}
	return nil
}
func (f *FakeSessionController) WaitClose() {
	if f.WaitCloseFunc != nil {
		f.WaitCloseFunc()
	}
}

func (f *FakeSessionController) Status() string {
	if f.StatusFunc != nil {
		return f.StatusFunc()
	}
	return ""
}

func (f *FakeSessionController) Close() error {
	if f.CloseFunc != nil {
		return f.CloseFunc()
	}
	return nil
}

func (f *FakeSessionController) Resize(args api.ResizeArgs) {
	if f.ResizeFunc != nil {
		f.ResizeFunc()
	}
}
