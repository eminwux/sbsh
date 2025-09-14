package session

import (
	"sbsh/pkg/api"
)

type FakeSessionController struct {
	Exit chan error

	AddedSpec *api.SessionSpec

	RunFunc       func()
	WaitReadyFunc func() error
	WaitCloseFunc func()
	StatusFunc    func() string
}

func (f *FakeSessionController) AddSession(s *api.SessionSpec) error {
	cp := *s
	f.AddedSpec = &cp
	return nil
}
func (f *FakeSessionController) Run() {
	if f.RunFunc != nil {
		f.RunFunc()
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
		f.StatusFunc()
	}
	return ""
}
