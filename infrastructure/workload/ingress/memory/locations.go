// Package memory keeps where the resources the ingress routes to are, in
// memory, which is the only place the ingress keeps it: every ingress hears
// everything said of them for itself, so what one knows is its nodes' word, a
// beat old at most, less what the commands on their way to them take away,
// and nothing it shares with another or keeps past a restart.
package memory

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Locations keeps what was heard of where each resource is, for as long as
// it goes on being heard.
type Locations struct {
	lock sync.RWMutex

	// kept is what is kept of each resource, by kind and by uuid; slugs is
	// the uuid each slug names, by kind.
	kept  map[string]map[string]*kept
	slugs map[string]map[string]string

	// prunedAt is when what went unheard for too long was last let go of.
	prunedAt time.Time

	forgetAfter time.Duration
	now         func() time.Time
}

var _ ingress.Locations = &Locations{}

// kept is what is kept of one resource.
type kept struct {
	// heard is where it was last heard to be, when known says it was heard
	// of at all.
	heard ingress.Heard
	known bool

	// said is when each node last said anything of it that was taken, by
	// the node's own clock.
	said map[string]time.Time

	// withheld is the command that took something away of it and has not
	// been answered, and withheldAt when it did, by this ingress's clock:
	// nothing when no command withholds anything.
	withheld   string
	withheldAt time.Time
}

// NewLocations is nothing heard yet, each resource to be forgotten once it
// has gone unheard for longer than forgetAfter, and what a command took
// away of one given back once it has gone unanswered for as long.
func NewLocations(forgetAfter time.Duration) *Locations {
	return &Locations{
		kept:        make(map[string]map[string]*kept),
		slugs:       make(map[string]map[string]string),
		forgetAfter: forgetAfter,
		now:         time.Now,
	}
}

// Hear keeps what a heartbeat said of where a resource is, unless a command
// withholds it.
func (l *Locations) Hear(_ context.Context, heard ingress.Heard) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.prune()

	if k, there := l.kept[heard.Kind][heard.UUID]; there && l.withholds(k) {
		return
	}

	l.take(heard)
}

// Withhold takes away what a command takes away of a resource, until its
// answer is heard. One nothing was heard of has nothing to take away, and,
// when the command takes it out of being reached, is given nothing by what
// is heard before its answer either.
func (l *Locations) Withhold(_ context.Context, withheld ingress.Withheld) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.prune()

	k, there := l.kept[withheld.Kind][withheld.UUID]
	if !there || !k.known || k.heard.Gone {
		if len(withheld.State) == 0 {
			return
		}

		k = l.keep(withheld.Kind, withheld.UUID)
		k.withheld, k.withheldAt = withheld.Command, l.now()

		return
	}

	// of the ports it was heard to let in, only those the command lets in too
	// are left; a command that leaves it reached on all of them takes
	// nothing away.
	left := slices.DeleteFunc(slices.Clone(k.heard.Ports), func(p port.Port) bool { return !slices.Contains(withheld.Ports, p) })

	if len(withheld.State) == 0 && len(left) == len(k.heard.Ports) {
		return
	}

	k.heard.Ports = left

	if len(withheld.State) > 0 {
		k.heard.State = withheld.State
	}

	k.withheld, k.withheldAt = withheld.Command, l.now()
}

// Answer gives back what the command answered withheld, and keeps where its
// result says it left the resource, as Hear keeps a heartbeat, unless a
// later command withholds it.
func (l *Locations) Answer(_ context.Context, answered ingress.Answered) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.prune()

	k, there := l.kept[answered.Kind][answered.UUID]
	if there && k.withheld == answered.Command {
		k.withheld = ""
	}

	if answered.Heard == nil || (there && l.withholds(k)) {
		return
	}

	l.take(*answered.Heard)
}

// ByUUID is what was last heard of the resource of the named kind uuid
// names, while it was heard lately.
func (l *Locations) ByUUID(_ context.Context, kindName string, uuid string) (ingress.Heard, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	k, there := l.kept[kindName][uuid]
	if !there || !l.found(k) {
		return ingress.Heard{}, fmt.Errorf("%w: no %s %q has been heard of lately", domain.ErrNotExists, kindName, uuid)
	}

	return clone(k.heard), nil
}

// BySlug is what was last heard of the resource of the named kind a slug
// names, while it was heard lately.
func (l *Locations) BySlug(_ context.Context, kindName string, slug string) (ingress.Heard, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	notThere := fmt.Errorf("%w: no %s has been heard of under %q lately", domain.ErrNotExists, kindName, slug)

	uuid, named := l.slugs[kindName][slug]
	if !named {
		return ingress.Heard{}, notThere
	}

	k, there := l.kept[kindName][uuid]
	if !there || !l.found(k) {
		return ingress.Heard{}, notThere
	}

	return clone(k.heard), nil
}

// take keeps what a node said of where a resource is, in place of what was
// heard of it before, unless the same node said something later already. Its
// slug names it from then on, unless another resource was heard under it
// later. It is called with the lock held.
func (l *Locations) take(heard ingress.Heard) {
	k := l.keep(heard.Kind, heard.UUID)

	if said, there := k.said[heard.Node]; there && heard.At.Before(said) {
		return
	}

	k.said[heard.Node] = heard.At

	previous, known := k.heard, k.known

	// a resource is reached under the slug it was given, which nothing
	// changes: one heard under none is under the one it was heard under.
	if len(heard.Slug) == 0 && known {
		heard.Slug = previous.Slug
	}

	heard.Ports = slices.Clone(heard.Ports)
	k.heard, k.known = heard, true

	if known && previous.Slug != heard.Slug {
		l.unslug(previous)
	}

	if heard.Gone {
		l.unslug(heard)

		return
	}

	if len(heard.Slug) == 0 {
		return
	}

	if other, named := l.slugs[heard.Kind][heard.Slug]; named && other != heard.UUID {
		if rival, heardOf := l.kept[heard.Kind][other]; heardOf && rival.known && heard.At.Before(rival.heard.At) {
			return
		}
	}

	l.slugs[heard.Kind][heard.Slug] = heard.UUID
}

// keep is what is kept of a resource, kept from now on if nothing was. It is
// called with the lock held.
func (l *Locations) keep(kindName string, uuid string) *kept {
	if l.kept[kindName] == nil {
		l.kept[kindName] = make(map[string]*kept)
		l.slugs[kindName] = make(map[string]string)
	}

	k, there := l.kept[kindName][uuid]
	if !there {
		k = &kept{said: make(map[string]time.Time)}
		l.kept[kindName][uuid] = k
	}

	return k
}

// found reports whether what is kept of a resource says where it is: it was
// heard of lately, and not as gone.
func (l *Locations) found(k *kept) bool {
	return k.known && !k.heard.Gone && !l.stale(k.heard.At)
}

// withholds reports whether a command still withholds what it took away of
// a resource: it has not been answered, and has not gone unanswered for as
// long as a resource may go unheard.
func (l *Locations) withholds(k *kept) bool {
	return len(k.withheld) > 0 && !l.stale(k.withheldAt)
}

// stale reports whether something that happened at was too long ago to be
// held to.
func (l *Locations) stale(at time.Time) bool {
	return l.now().Sub(at) > l.forgetAfter
}

// prune lets go of what has gone unheard for too long, now and then, unless
// a command still withholds something of it: what is not found any more is
// not kept either. It is called with the lock held.
func (l *Locations) prune() {
	now := l.now()
	if now.Sub(l.prunedAt) < l.forgetAfter {
		return
	}

	l.prunedAt = now

	for _, byUUID := range l.kept {
		for uuid, k := range byUUID {
			if (k.known && !l.stale(k.heard.At)) || l.withholds(k) {
				continue
			}

			delete(byUUID, uuid)

			if k.known {
				l.unslug(k.heard)
			}
		}
	}
}

// unslug lets go of the slug what was heard was heard under, while that
// still names it. It is called with the lock held.
func (l *Locations) unslug(heard ingress.Heard) {
	if uuid, named := l.slugs[heard.Kind][heard.Slug]; named && uuid == heard.UUID {
		delete(l.slugs[heard.Kind], heard.Slug)
	}
}

// clone is what was heard, sharing nothing with what is kept.
func clone(heard ingress.Heard) ingress.Heard {
	heard.Ports = slices.Clone(heard.Ports)

	return heard
}
