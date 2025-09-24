package sessionstore

import (
	"errors"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
)

type SessionStoreTest struct {
	// Last-call trackers (useful for assertions)
	LastAdded        *SupervisedSession
	LastGetID        api.ID
	LastRemovedID    api.ID
	LastSetCurrentID api.ID

	// Optional: store a value to be returned by Current() when CurrentFunc is nil
	CurrentID api.ID

	// Stub functions (set these in tests to control behavior)
	AddFunc        func(s *SupervisedSession) error
	GetFunc        func(id api.ID) (*SupervisedSession, bool)
	ListLiveFunc   func() []api.ID
	RemoveFunc     func(id api.ID)
	CurrentFunc    func() api.ID
	SetCurrentFunc func(id api.ID) error
}

func NewSessionStoreTest() *SessionStoreTest {
	return &SessionStoreTest{
		AddFunc: func(s *SupervisedSession) error {
			if s == nil {
				return errors.New("nil session")
			}
			return nil
		},
		GetFunc: func(id api.ID) (*SupervisedSession, bool) {
			if id == "" {
				return nil, false
			}
			return &SupervisedSession{Id: id}, true
		},
		ListLiveFunc: func() []api.ID {
			return []api.ID{"sess-1", "sess-2"}
		},
		RemoveFunc: func(id api.ID) {
			// no-op, LastRemovedID is tracked automatically
		},
		CurrentFunc: func() api.ID {
			return "sess-1"
		},
		SetCurrentFunc: func(id api.ID) error {
			if id == "" {
				return errors.New("cannot set empty id")
			}
			return nil
		},
	}
}

func (t *SessionStoreTest) Add(s *SupervisedSession) error {
	t.LastAdded = s
	if t.AddFunc != nil {
		return t.AddFunc(s)
	}
	return errdefs.ErrFuncNotSet
}

func (t *SessionStoreTest) Get(id api.ID) (*SupervisedSession, bool) {
	t.LastGetID = id
	if t.GetFunc != nil {
		return t.GetFunc(id)
	}
	return nil, false
}

func (t *SessionStoreTest) ListLive() []api.ID {
	if t.ListLiveFunc != nil {
		return t.ListLiveFunc()
	}
	return nil
}

func (t *SessionStoreTest) Remove(id api.ID) {
	t.LastRemovedID = id
	if t.RemoveFunc != nil {
		t.RemoveFunc(id)
	}
}

func (t *SessionStoreTest) Current() api.ID {
	if t.CurrentFunc != nil {
		return t.CurrentFunc()
	}
	return t.CurrentID
}

func (t *SessionStoreTest) SetCurrent(id api.ID) error {
	t.LastSetCurrentID = id
	if t.SetCurrentFunc != nil {
		return t.SetCurrentFunc(id)
	}
	return errdefs.ErrFuncNotSet
}
