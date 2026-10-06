package blocks

import (
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Sightings are what the nodes last reported of the building blocks nobody
// keeps a record of, by kind and by the Docker VM each is in: what a stack or
// a VM's terminal made.
//
// They are what a heartbeat said, not records: each control plane keeps what
// the heartbeats it hears say, and every heartbeat says it all again, so a
// control plane that has just started, or that has not heard from a node for
// a moment, shows what it heard last. What was seen longer ago than
// staleAfter is not shown at all.
type Sightings struct {
	lock sync.RWMutex

	// seen is what was seen, by kind and by Docker VM.
	seen map[string]map[string]Seen

	staleAfter time.Duration
	now        func() time.Time
}

// Seen is what a node saw in one Docker VM.
type Seen struct {
	Node string
	At   time.Time

	// Manifests are what it saw there that nobody keeps a record of, as
	// manifests of the kind.
	Manifests []kind.Raw
}

// staleAfter is how long what was seen is shown for, once its node has
// stopped saying it: a node falls silent after half a minute.
const staleAfter = time.Minute

// NewSightings is nothing seen yet.
func NewSightings() *Sightings {
	return &Sightings{seen: make(map[string]map[string]Seen), staleAfter: staleAfter, now: time.Now}
}

// Saw keeps what a node saw in one Docker VM of the named kind, in place of
// what was seen there before.
func (s *Sightings) Saw(kindName string, vmUUID string, seen Seen) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.seen[kindName] == nil {
		s.seen[kindName] = make(map[string]Seen)
	}

	s.seen[kindName][vmUUID] = seen
}

// Forget lets go of what was seen of the named kind in the Docker VMs of a
// node that keep tells to keep no longer.
func (s *Sightings) Forget(kindName string, nodeName string, keep func(vmUUID string) bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for vmUUID, seen := range s.seen[kindName] {
		if seen.Node == nodeName && !keep(vmUUID) {
			delete(s.seen[kindName], vmUUID)
		}
	}
}

// All is everything of the named kind seen lately, newest first.
func (s *Sightings) All(kindName string) []kind.Raw {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var all []kind.Raw

	for _, vmUUID := range slices.Sorted(maps.Keys(s.seen[kindName])) {
		seen := s.seen[kindName][vmUUID]
		if s.stale(seen) {
			continue
		}

		all = append(all, seen.Manifests...)
	}

	slices.SortStableFunc(all, newestFirst)

	return all
}

// One is what of the named kind uuid names, if it was seen lately.
func (s *Sightings) One(kindName string, uuid string) (kind.Raw, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for _, seen := range s.seen[kindName] {
		if s.stale(seen) {
			continue
		}

		for _, manifest := range seen.Manifests {
			if manifest.Metadata.UUID == uuid {
				return manifest, true
			}
		}
	}

	return kind.Raw{}, false
}

// In is what of the named kind was seen lately in one Docker VM.
func (s *Sightings) In(kindName string, vmUUID string) []kind.Raw {
	s.lock.RLock()
	defer s.lock.RUnlock()

	seen, ok := s.seen[kindName][vmUUID]
	if !ok || s.stale(seen) {
		return nil
	}

	return slices.Clone(seen.Manifests)
}

// Changed has what of the named kind uuid names be as after says, once a
// command changed it, or gone when after is nothing: until the next heartbeat
// says so too.
func (s *Sightings) Changed(kindName string, uuid string, after *kind.Raw) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for vmUUID, seen := range s.seen[kindName] {
		i := slices.IndexFunc(seen.Manifests, func(m kind.Raw) bool { return m.Metadata.UUID == uuid })
		if i < 0 {
			continue
		}

		manifests := slices.Clone(seen.Manifests)

		if after == nil {
			manifests = slices.Delete(manifests, i, i+1)
		} else {
			manifests[i] = *after
		}

		seen.Manifests = manifests
		s.seen[kindName][vmUUID] = seen

		return
	}
}

// stale reports whether what was seen was seen too long ago to show.
func (s *Sightings) stale(seen Seen) bool {
	return s.now().Sub(seen.At) > s.staleAfter
}

// newestFirst orders manifests by when they were made, the newest first, and
// of two made at the same moment by their uuids.
func newestFirst(a kind.Raw, b kind.Raw) int {
	if order := b.Metadata.CreatedAt.Compare(a.Metadata.CreatedAt); order != 0 {
		return order
	}

	return strings.Compare(b.Metadata.UUID, a.Metadata.UUID)
}
