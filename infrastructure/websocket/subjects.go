package websocket

import "sync"

// subjects is the set of subjects the websocket consumes. Clients may only send
// requests to a subject in the set.
type subjects struct {
	lock sync.RWMutex
	set  map[string]struct{}
}

func newSubjects() *subjects {
	return &subjects{
		set: make(map[string]struct{}),
	}
}

func (s *subjects) add(subject string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set[subject] = struct{}{}
}

func (s *subjects) has(subject string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.set[subject]

	return ok
}
