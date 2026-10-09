package blocks

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
)

// Sightings are what the nodes last said of the building blocks nobody keeps
// a record of, by kind and by uuid: what a stack or a VM's terminal made.
//
// They are what heartbeats said, not records: each control plane keeps what
// the heartbeats it hears say, one thing in each, and every beat says it all
// again, so a control plane that has just started, or that has not heard from
// a node for a moment, shows what it heard last. No heartbeat says a thing is
// gone, so what was seen longer ago than they are shown for is not shown at
// all: that is how what went from its VM's dockerd, or what lives in a VM that
// stopped, is let go of.
type Sightings struct {
	lock sync.RWMutex

	// seen is what was seen, by kind and by uuid.
	seen map[string]map[string]Seen

	// prunedAt is when what went stale was last let go of.
	prunedAt time.Time

	staleAfter time.Duration
	now        func() time.Time
}

// Seen is what a node saw of one thing nobody keeps a record of.
type Seen struct {
	Node string
	At   time.Time

	// Manifest is what it saw, as a manifest of its kind, in the Docker VM
	// among its owners.
	Manifest kind.Raw

	// gone says a command took it away: what was seen of it before then is
	// not shown again.
	gone bool
}

// NewSightings is nothing seen yet, each thing to be shown for as long as
// shownFor once its node has stopped saying it.
func NewSightings(shownFor time.Duration) *Sightings {
	return &Sightings{seen: make(map[string]map[string]Seen), staleAfter: shownFor, now: time.Now}
}

// Saw keeps what a node saw of one thing of the named kind, in place of what
// was seen of it before, unless that was seen later, or a command took it away
// later.
func (s *Sightings) Saw(kindName string, seen Seen) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.prune()

	uuid := seen.Manifest.Metadata.UUID

	if kept, there := s.seen[kindName][uuid]; there && seen.At.Before(kept.At) {
		return
	}

	if s.seen[kindName] == nil {
		s.seen[kindName] = make(map[string]Seen)
	}

	s.seen[kindName][uuid] = seen
}

// Forget lets go of what was seen of the named kind under uuid: a record
// speaks for it now.
func (s *Sightings) Forget(kindName string, uuid string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.seen[kindName], uuid)
}

// All is everything of the named kind seen lately, newest first.
func (s *Sightings) All(kindName string) []kind.Raw {
	return s.shown(kindName, func(kind.Raw) bool { return true })
}

// One is what of the named kind uuid names, if it was seen lately.
func (s *Sightings) One(kindName string, uuid string) (kind.Raw, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	seen, there := s.seen[kindName][uuid]
	if !there || !s.shows(seen) {
		return kind.Raw{}, false
	}

	return seen.Manifest, true
}

// In is what of the named kind was seen lately in one Docker VM, newest
// first.
func (s *Sightings) In(kindName string, vmUUID string) []kind.Raw {
	return s.shown(kindName, func(manifest kind.Raw) bool {
		vm, in := manifest.Metadata.Owner(blockKinds.Parent)

		return in && vm.UUID == vmUUID
	})
}

// Changed has what of the named kind uuid names be as after says, once a
// command changed it, or gone when after is nothing: until a heartbeat taken
// since says otherwise.
func (s *Sightings) Changed(kindName string, uuid string, after *kind.Raw) {
	s.lock.Lock()
	defer s.lock.Unlock()

	seen, there := s.seen[kindName][uuid]
	if !there {
		return
	}

	seen.At = s.now()

	if after == nil {
		seen.gone = true
	} else {
		seen.Manifest = *after
	}

	s.seen[kindName][uuid] = seen
}

// shown is what of the named kind that is lets through was seen lately,
// newest first.
func (s *Sightings) shown(kindName string, is func(kind.Raw) bool) []kind.Raw {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var shown []kind.Raw

	for _, seen := range s.seen[kindName] {
		if s.shows(seen) && is(seen.Manifest) {
			shown = append(shown, seen.Manifest)
		}
	}

	slices.SortFunc(shown, newestFirst)

	return shown
}

// shows reports whether what was seen is shown: seen lately, and not taken
// away since.
func (s *Sightings) shows(seen Seen) bool {
	return !seen.gone && !s.stale(seen)
}

// stale reports whether what was seen was seen too long ago to show.
func (s *Sightings) stale(seen Seen) bool {
	return s.now().Sub(seen.At) > s.staleAfter
}

// prune lets go of what went stale, now and then: what is not shown any more
// is not kept either.
func (s *Sightings) prune() {
	now := s.now()
	if now.Sub(s.prunedAt) < s.staleAfter {
		return
	}

	s.prunedAt = now

	for _, seen := range s.seen {
		for uuid, each := range seen {
			if s.stale(each) {
				delete(seen, uuid)
			}
		}
	}
}

// newestFirst orders manifests by when they were made, the newest first, and
// of two made at the same moment by their uuids.
func newestFirst(a kind.Raw, b kind.Raw) int {
	if order := b.Metadata.CreatedAt.Compare(a.Metadata.CreatedAt); order != 0 {
		return order
	}

	return strings.Compare(b.Metadata.UUID, a.Metadata.UUID)
}
