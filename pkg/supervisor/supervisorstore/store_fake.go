package supervisorstore

import (
	"errors"
	"sbsh/pkg/api"
)

type SessionManagerTest struct {
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

func NewSessionManagerTest() *SessionManagerTest {
	return &SessionManagerTest{
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

func (t *SessionManagerTest) Add(s *SupervisedSession) error {
	t.LastAdded = s
	if t.AddFunc != nil {
		return t.AddFunc(s)
	}
	return ErrFuncNotSet
}

func (t *SessionManagerTest) Get(id api.SessionID) (*SupervisedSession, bool) {
	t.LastGetID = id
	if t.GetFunc != nil {
		return t.GetFunc(id)
	}
	return nil, false
}

func (t *SessionManagerTest) ListLive() []api.SessionID {
	if t.ListLiveFunc != nil {
		return t.ListLiveFunc()
	}
	return nil
}

func (t *SessionManagerTest) Remove(id api.SessionID) {
	t.LastRemovedID = id
	if t.RemoveFunc != nil {
		t.RemoveFunc(id)
	}
}

func (t *SessionManagerTest) Current() api.SessionID {
	if t.CurrentFunc != nil {
		return t.CurrentFunc()
	}
	return t.CurrentID
}

func (t *SessionManagerTest) SetCurrent(id api.SessionID) error {
	t.LastSetCurrentID = id
	if t.SetCurrentFunc != nil {
		return t.SetCurrentFunc(id)
	}
	return ErrFuncNotSet
}
