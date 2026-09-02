package attachContainer

import (
	"sync"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// terminals is the set of terminals this replica has open, so that what a
// client types reaches the one it typed into.
//
// A terminal only exists on the replica that opened it, and input is produced
// onto the broker, so exactly one replica is handed each keystroke. With more
// than one replica serving the dashboard, that need not be the one holding the
// terminal — so a terminal is a single-replica feature until input travels the
// way replies do, published to every replica rather than produced to one.
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
