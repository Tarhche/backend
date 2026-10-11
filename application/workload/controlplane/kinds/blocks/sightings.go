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
// a record of, by kind and by uuid: what a VM's terminal made.
//
// They are what heartbeats said, not records: each control plane keeps what
// the heartbeats it hears say, one thing in each, and every beat says it all
// again, so a control plane that has just started, or that has not heard from
// a node for a moment, shows what it heard last. No heartbeat says a thing is
// gone, so what was seen longer ago than they are shown for is not shown at
// all: that is how what went from its VM's dockerd, or what lives in a VM that
// stopped, is let go of.
//
// What the platform labelled as its own (Seen.Labelled) is shown only once
// its node has gone on saying it for long enough, by the node's own clock:
// from the first heartbeat of a run of them, each said soon enough after the
// one before, to the latest. A heartbeat its node took before it removed a
// container, a network or a volume, heard once the delete's result has
// deleted its record, says it of a moment before it went, and nothing says it
// after: its run is only as long as the heartbeats that were on their way when
// the record went, far less than a record may go unheard, and it is never
// shown, here or by another control plane, which hears other heartbeats of
// it. What outlived its record, or is on a disk restored long ago, is said
// beat after beat, and is shown once it has been said for long enough.
//
// Nor is it shown once its node has gone on saying what else its Docker VM
// holds, as heard here, for as long without a word of it (Heard): heartbeats
// of one kind heard so late that they alone span as long, a backlog of them
// taken before a delete removed what they say, leave it behind the node's
// other heartbeats of that VM and that beat on, which are heard in time. A
// Docker VM whose dockerd answers says something every beat, its default
// networks if nothing else, and one whose dockerd does not answer says
// nothing, which leaves nothing of it behind: what it holds is shown until
// it is stale, whatever the node says of its other VMs. A backlog of every
// heartbeat a node sends alike, as long as a record may go unheard, tells
// nothing apart, and what it says is shown until it is stale: a ghost for as
// long as twice that, from the last word of it, by the node's clock.
type Sightings struct {
	lock sync.RWMutex

	// seen is what was seen, by kind and by uuid.
	seen map[string]map[string]Seen

	// prunedAt is when what went stale was last let go of.
	prunedAt time.Time

	staleAfter time.Duration

	// sureAfter is how long a node goes on saying what the platform
	// labelled as its own before it is shown, and how long it may go
	// without a word of it in between.
	sureAfter time.Duration

	// heard is the latest moment each node said what each of its Docker
	// VMs holds as of, by its own clock, in a heartbeat heard here, of any
	// kind.
	heard map[where]time.Time

	now func() time.Time
}

// where is a Docker VM on a node, as a node says what it holds.
type where struct {
	node string
	vm   string
}

// Seen is what a node saw of one thing nobody keeps a record of.
type Seen struct {
	Node string
	At   time.Time

	// Manifest is what it saw, as a manifest of its kind, in the Docker VM
	// among its owners.
	Manifest kind.Raw

	// Labelled says the platform labelled it as one of its own, under the
	// uuid it is seen by (blockKinds.Identity): what a record kept until a
	// moment ago, as a heartbeat its node took before the record's delete
	// removed it says, or what outlived its record. It is shown only once
	// its node has gone on saying it for long enough.
	Labelled bool

	// since and said are when its node first and last said it, by the
	// node's clock, in a run of heartbeats each said soon enough after the
	// one before: how long it has been said for. What a command changed of
	// it, as of the control plane's clock (At), is kept apart from them.
	since time.Time
	said  time.Time

	// gone says a command took it away: what was seen of it before then is
	// not shown again.
	gone bool
}

// NewSightings is nothing seen yet, each thing to be shown for as long as
// shownFor once its node has stopped saying it, and what the platform
// labelled as its own only once its node has gone on saying it for as long
// as sureAfter, never as long again without a word of it.
func NewSightings(shownFor time.Duration, sureAfter time.Duration) *Sightings {
	return &Sightings{seen: make(map[string]map[string]Seen), heard: make(map[where]time.Time), staleAfter: shownFor, sureAfter: sureAfter, now: time.Now}
}

// Heard keeps that a node said what its Docker VM vmUUID names holds as of a
// moment, by its own clock, in a heartbeat heard here, of whatever kind, a
// record kept of it or not: what the platform labelled as its own in that VM
// that its node goes on that long after without a word of is not shown.
func (s *Sightings) Heard(nodeName string, vmUUID string, at time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()

	in := where{node: nodeName, vm: vmUUID}
	if at.After(s.heard[in]) {
		s.heard[in] = at
	}
}

// Saw keeps what a node saw of one thing of the named kind, in place of what
// was seen of it before, unless that was seen later, or a command took it away
// later. It is said for as long as its node has said it without going for
// longer than sureAfter without a word of it: what that node said of it before
// such a gap, what another node said of it, by another clock, and what a
// command took away since, start no run of this one.
func (s *Sightings) Saw(kindName string, seen Seen) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.prune()

	uuid := seen.Manifest.Metadata.UUID

	kept, there := s.seen[kindName][uuid]
	if there && seen.At.Before(kept.At) {
		return
	}

	seen.since, seen.said = seen.At, seen.At
	if there && !kept.gone && kept.Node == seen.Node && seen.At.Sub(kept.said) <= s.sureAfter {
		seen.since = kept.since
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
	return s.shown(kindName, s.shows, func(kind.Raw) bool { return true })
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
	return s.shown(kindName, s.shows, in(vmUUID))
}

// Held is what of the named kind its node says one Docker VM holds, newest
// first: what is shown there (In), and what the platform labelled as its own
// that its node has not said for long enough yet to be shown, which is there
// all the same, as what outlived its record is, or anything a control plane
// that has just started hears. What a command took away, what went stale, and
// what its node left behind (Heard) are not. Not showing what is labelled at
// once is for showing alone: what asks whether a VM's dockerd holds a thing
// by its name, as docker would answer it, asks Held. A heartbeat its node took
// before a delete removed something, heard once its record went, says it is
// held for as long as its node is not heard to have left it behind.
func (s *Sightings) Held(kindName string, vmUUID string) []kind.Raw {
	return s.shown(kindName, s.holds, in(vmUUID))
}

// in is what lives in the Docker VM vmUUID names.
func in(vmUUID string) func(kind.Raw) bool {
	return func(manifest kind.Raw) bool {
		vm, in := manifest.Metadata.Owner(blockKinds.Parent)

		return in && vm.UUID == vmUUID
	}
}

// Changed has what of the named kind uuid names be as after says, once a
// command changed it, or gone when after is nothing: until a heartbeat taken
// since says otherwise. How long its node has said it for is as it was.
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

// shown is what of the named kind that is lets through was seen lately, as
// what visible says of each, newest first.
func (s *Sightings) shown(kindName string, visible func(Seen) bool, is func(kind.Raw) bool) []kind.Raw {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var shown []kind.Raw

	for _, seen := range s.seen[kindName] {
		if visible(seen) && is(seen.Manifest) {
			shown = append(shown, seen.Manifest)
		}
	}

	slices.SortFunc(shown, newestFirst)

	return shown
}

// shows reports whether what was seen is shown: seen lately, not taken away
// since, and, when the platform labelled it as its own, said for long enough,
// and not left behind by its node.
func (s *Sightings) shows(seen Seen) bool {
	return !seen.gone && !s.stale(seen) && (!seen.Labelled || (seen.said.Sub(seen.since) >= s.sureAfter && !s.left(seen)))
}

// holds reports whether what was seen is held by its VM's dockerd, as its
// node last said, shown or not yet: seen lately, not taken away since, and,
// when the platform labelled it as its own, not left behind by its node.
func (s *Sightings) holds(seen Seen) bool {
	return !seen.gone && !s.stale(seen) && (!seen.Labelled || !s.left(seen))
}

// left reports whether what was seen was left behind by its node: it went on
// saying what else the Docker VM it is in holds, as heard here, for
// sureAfter since it last said it.
func (s *Sightings) left(seen Seen) bool {
	vm, _ := seen.Manifest.Metadata.Owner(blockKinds.Parent)

	return s.heard[where{node: seen.Node, vm: vm.UUID}].Sub(seen.said) >= s.sureAfter
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

	// nor when a node last said what a Docker VM holds, once that is as
	// long ago: a VM gone, or stopped, whose sightings went stale with it.
	for in, at := range s.heard {
		if now.Sub(at) > s.staleAfter {
			delete(s.heard, in)
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
