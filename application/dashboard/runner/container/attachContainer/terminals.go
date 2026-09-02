package attachContainer

import (
	"sync"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// terminals is the set of terminals this replica has open, so that what a
// client types reaches the one it typed into.
//
// A terminal is only ever reachable from the replica holding it. A client's
// input is produced onto the broker and handled by one replica, which may not
// be this one; a replica that does not have the terminal simply has nothing to
// write to, and the one that does writes it.
type terminals struct {
	lock sync.RWMutex
	open map[string]runnerManager.Attachment
}

func newTerminals() *terminals {
	return &terminals{open: make(map[string]runnerManager.Attachment)}
}

func (t *terminals) add(requestID string, attachment runnerManager.Attachment) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.open[requestID] = attachment
}

func (t *terminals) get(requestID string) (runnerManager.Attachment, bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	attachment, ok := t.open[requestID]

	return attachment, ok
}

func (t *terminals) remove(requestID string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.open, requestID)
}
