package vmhost

import "sync"

// keyedLock serialises what is done to one VM, and nothing else. A VM's lock
// lives only while somebody holds it or waits for it, so locking IDs nobody
// will ever lock again costs nothing afterwards.
type keyedLock struct {
	guard sync.Mutex
	held  map[string]*keyed
}

type keyed struct {
	sync.Mutex

	// refs is how many hold the lock or wait for it.
	refs int
}

// lock waits for the VM's lock, and returns what lets go of it.
func (k *keyedLock) lock(key string) func() {
	entry := k.acquire(key)
	entry.Lock()

	return func() { k.release(key, entry) }
}

// tryLock takes the VM's lock if nobody holds it, and reports whether it did.
// It is how reconciling passes over a VM that something is being done to: what
// is being done to it is that VM's account of itself.
func (k *keyedLock) tryLock(key string) (func(), bool) {
	entry := k.acquire(key)

	if !entry.TryLock() {
		k.drop(key, entry)

		return nil, false
	}

	return func() { k.release(key, entry) }, true
}

func (k *keyedLock) acquire(key string) *keyed {
	k.guard.Lock()
	defer k.guard.Unlock()

	if k.held == nil {
		k.held = make(map[string]*keyed)
	}

	entry, found := k.held[key]
	if !found {
		entry = &keyed{}
		k.held[key] = entry
	}

	entry.refs++

	return entry
}

func (k *keyedLock) release(key string, entry *keyed) {
	entry.Unlock()
	k.drop(key, entry)
}

func (k *keyedLock) drop(key string, entry *keyed) {
	k.guard.Lock()
	defer k.guard.Unlock()

	entry.refs--
	if entry.refs == 0 {
		delete(k.held, key)
	}
}
