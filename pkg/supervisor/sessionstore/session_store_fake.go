package sessionstore

import (
	"errors"
	"sbsh/pkg/api"
)

type SessionStoreTest struct {
	// Last-call trackers (useful for assertions)
	LastAdded        *SupervisedSession
	LastGetID        api.SessionID
	LastRemovedID    api.SessionID
	LastSetCurrentID api.SessionID

	// Optional: store a value to be returned by Current() when CurrentFunc is nil
	CurrentID api.SessionID

	// Stub functions (set these in tests to control behavior)
	AddFunc        func(s *SupervisedSession) error
	GetFunc        func(id api.SessionID) (*SupervisedSession, bool)
	ListLiveFunc   func() []api.SessionID
	RemoveFunc     func(id api.SessionID)
	CurrentFunc    func() api.SessionID
	SetCurrentFunc func(id api.SessionID) error
}

func NewSessionStoreTest() *SessionStoreTest {
	return &SessionStoreTest{
		AddFunc: func(s *SupervisedSession) error {
			if s == nil {
				return errors.New("nil session")
			}
			return nil
		},
		GetFunc: func(id api.SessionID) (*SupervisedSession, bool) {
			if id == "" {
				return nil, false
			}
			return &SupervisedSession{Id: id}, true
		},
		ListLiveFunc: func() []api.SessionID {
			return []api.SessionID{"sess-1", "sess-2"}
		},
		RemoveFunc: func(id api.SessionID) {
			// no-op, LastRemovedID is tracked automatically
		},
		CurrentFunc: func() api.SessionID {
			return "sess-1"
		},
		SetCurrentFunc: func(id api.SessionID) error {
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
	return ErrFuncNotSet
}

func (t *SessionStoreTest) Get(id api.SessionID) (*SupervisedSession, bool) {
	t.LastGetID = id
	if t.GetFunc != nil {
		return t.GetFunc(id)
	}
	return nil, false
}

func (t *SessionStoreTest) ListLive() []api.SessionID {
	if t.ListLiveFunc != nil {
		return t.ListLiveFunc()
	}
	return nil
}

func (t *SessionStoreTest) Remove(id api.SessionID) {
	t.LastRemovedID = id
	if t.RemoveFunc != nil {
		t.RemoveFunc(id)
	}
}

func (t *SessionStoreTest) Current() api.SessionID {
	if t.CurrentFunc != nil {
		return t.CurrentFunc()
	}
	return t.CurrentID
}

func (t *SessionStoreTest) SetCurrent(id api.SessionID) error {
	t.LastSetCurrentID = id
	if t.SetCurrentFunc != nil {
		return t.SetCurrentFunc(id)
	}
	return ErrFuncNotSet
}
