// Package watch keeps the dashboard's listings as they are, without asking for
// them again.
//
// A client opens a watch and is told what changes: the runner already says what
// becomes of its tasks — every node reports the ones it holds, and the
// control plane says when one is scheduled, fails or is taken away — so a watch is
// those reports, turned into replies for whoever is looking at them. Nothing is
// polled, and nothing follows anything from the runner.
//
// Every replica hears every report and answers for the clients it is holding,
// which is why a watch registers here rather than anywhere shared: a replica
// always knows its own.
package watch

import "sync"

// everybody is the owner a watch has when it is over everybody's tasks,
// rather than one person's own.
const everybody = ""

// Watchers is who is being told what changes. A watch is registered under the
// request that opened it, which is what a reply is addressed to, and the person
// it is for, which is what a reply is filtered by.
type Watchers struct {
	lock   sync.RWMutex
	tasks  map[string]string
	stacks map[string]string
}

func NewWatchers() *Watchers {
	return &Watchers{
		tasks:  make(map[string]string),
		stacks: make(map[string]string),
	}
}

// WatchTasks follows every task, or one person's own when ownerUUID
// names somebody.
func (w *Watchers) WatchTasks(requestID string, ownerUUID string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.tasks[requestID] = ownerUUID
}

// WatchStacks follows every stack, or one person's own.
func (w *Watchers) WatchStacks(requestID string, ownerUUID string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.stacks[requestID] = ownerUUID
}

// Remove forgets the watch a request opened, whichever it was.
func (w *Watchers) Remove(requestID string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	delete(w.tasks, requestID)
	delete(w.stacks, requestID)
}

// Tasks is who to tell about a task that belongs to ownerUUID:
// everybody watching all of them, and whoever is watching their own when the
// task is theirs.
func (w *Watchers) Tasks(ownerUUID string) []string {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return addressees(w.tasks, ownerUUID)
}

// Stacks is the same, for a stack.
func (w *Watchers) Stacks(ownerUUID string) []string {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return addressees(w.stacks, ownerUUID)
}

// EveryTaskWatch is who to tell about a task whose owner nothing
// says: one that is gone names only which one it was, and a watch that never
// saw it makes nothing of being told.
func (w *Watchers) EveryTaskWatch() []string {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return all(w.tasks)
}

// EveryStackWatch is the same, for a stack that is gone.
func (w *Watchers) EveryStackWatch() []string {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return all(w.stacks)
}

// Watching reports whether anybody is watching anything here, which is what
// says a report is worth reading at all.
func (w *Watchers) Watching() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return len(w.tasks) > 0 || len(w.stacks) > 0
}

// all is every watch there is, whosever it follows.
func all(watches map[string]string) []string {
	requests := make([]string, 0, len(watches))

	for requestID := range watches {
		requests = append(requests, requestID)
	}

	return requests
}

func addressees(watches map[string]string, ownerUUID string) []string {
	requests := make([]string, 0, len(watches))

	for requestID, watching := range watches {
		if watching == everybody || (len(ownerUUID) > 0 && watching == ownerUUID) {
			requests = append(requests, requestID)
		}
	}

	return requests
}
