package watch

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"
)

// maxRemembered bounds what a replica keeps. Tasks come and go, and a
// watch that is told again about a change it has already seen costs a client
// nothing, while a map that only grows costs the replica holding it.
const maxRemembered = 10_000

// seen is what each task was last reported to be, so that a report saying
// exactly what the last one said is not passed on: a node reports every
// task it holds, over and over, and almost all of it is the same news.
//
// It is also where a task's owner and stack are kept, because what becomes
// of one — it failed, it is gone — says only which task it was.
type seen struct {
	lock  sync.Mutex
	tasks map[string]remembered
}

type remembered struct {
	digest    [sha256.Size]byte
	ownerUUID string
	stackUUID string
}

func newSeen() *seen {
	return &seen{tasks: make(map[string]remembered)}
}

// changed records what a task is now and reports whether that is news.
// A task nothing was remembered about is always news.
func (s *seen) changed(uuid string, task *Task, ownerUUID string, stackUUID string) bool {
	digest, err := fingerprint(task)
	if err != nil {
		return true
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.tasks) >= maxRemembered {
		clear(s.tasks)
	}

	previous, known := s.tasks[uuid]

	s.tasks[uuid] = remembered{digest: digest, ownerUUID: ownerUUID, stackUUID: stackUUID}

	return !known || previous.digest != digest
}

// of is what is remembered about a task, and nothing when it is one this
// replica has not been told about.
func (s *seen) of(uuid string) remembered {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.tasks[uuid]
}

// forget drops a task that is gone, and reports what was remembered about
// it: which stack it leaves is the last thing it has to say.
func (s *seen) forget(uuid string) remembered {
	s.lock.Lock()
	defer s.lock.Unlock()

	was := s.tasks[uuid]
	delete(s.tasks, uuid)

	return was
}

// fingerprint is what a task looks like now, without the moment it was
// reported: every beat is a different moment, and almost none of them are a
// different task.
func fingerprint(task *Task) ([sha256.Size]byte, error) {
	without := *task
	without.At = time.Time{}

	payload, err := json.Marshal(&without)
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	return sha256.Sum256(payload), nil
}
