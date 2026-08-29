package protocol

import "sync"

// Subjects is the set of subjects the websocket consumes. Clients may only send
// requests to a subject in the set.
type Subjects struct {
	lock sync.RWMutex
	set  map[string]struct{}
}

func NewSubjects() *Subjects {
	return &Subjects{
		set: make(map[string]struct{}),
	}
}

func (s *Subjects) Add(subject string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set[subject] = struct{}{}
}

func (s *Subjects) Has(subject string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.set[subject]

	return ok
}

// SubjectPrefix namespaces the websocket's subjects on the broker, keeping them
// clear of the subjects the rest of the system uses.
const SubjectPrefix = "websocket_"

// BrokerSubject is the subject a client-facing name is carried under.
func BrokerSubject(name string) string {
	return SubjectPrefix + name
}
