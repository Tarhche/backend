package observe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// tries is how many times a record is read and written again when
	// something else wrote it in the meantime.
	tries = 3

	// refreshAfter is how long a resource whose node keeps saying the same
	// thing goes before that is written down again, unless the observer is
	// told otherwise (WithRefresh): often enough that when it was last
	// observed stays roughly true, rarely enough that a heartbeat is not a
	// write for every resource every second.
	refreshAfter = 15 * time.Second
)

// Parents say what the resources others live in are doing, when what lives
// in them cannot be looked at because of it: a Docker VM that is stopped,
// whose containers nobody can read until it runs again; and when what lives
// in one restored from a snapshot was first looked into since.
type Parents interface {
	// Down is the state parent is in, in its kind's own word, when what lives
	// in it cannot be looked at because of that state; and nothing when it
	// can be, or when that is not known, and what lives in it is then judged
	// as anything else is.
	Down(ctx context.Context, parent kind.Reference) (kind.State, error)

	// Looked is when what lives in parent was first looked into since the
	// parent was last restored from a snapshot, none when it was not since,
	// and whether parent says so at all: a Docker VM does
	// (vmKind.Status.LookedAt). What lives in a parent that says nothing of
	// it is judged as anything else is.
	Looked(ctx context.Context, parent kind.Reference) (time.Time, bool, error)
}

// Orphans are told of what a node holds that nobody keeps a record of: a VM
// deleted while its node could not be told, or made again by a command
// carried out after its delete was. Nobody is going to want it.
type Orphans interface {
	// Orphaned is told that nodeName holds a resource of d's kind, by uuid,
	// that has no record, as a heartbeat its node took at a moment, by its
	// own clock, says: one it took before the record's delete removed the
	// resource is heard after the record went now and then.
	Orphaned(ctx context.Context, d kind.Descriptor, nodeName string, uuid string, at time.Time) error

	// Kept is told that a heartbeat of nodeName's says of a resource of d's
	// kind, by uuid, that somebody keeps a record of: what its node said of
	// it with none before then, heard late once a delete took its record
	// away and before its graph made it again under its uuid, was said of no
	// orphan.
	Kept(ctx context.Context, d kind.Descriptor, nodeName string, uuid string)
}

// Observer writes down what the nodes' heartbeats say of the resources of
// every kind the control plane runs, and what their silence says.
type Observer struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	parents   Parents
	orphans   Orphans
	notifier  graph.Notifier
	refresh   time.Duration
	logger    *slog.Logger
}

// Option changes how an observer goes about it.
type Option func(*Observer)

// WithParents has what lives inside a parent that is down observed waiting
// on it, as parents say the parent is, when its node goes on without a word
// of it.
func WithParents(parents Parents) Option {
	return func(o *Observer) {
		o.parents = parents
	}
}

// WithOrphans has orphans told of what a node holds that nobody keeps a
// record of.
func WithOrphans(orphans Orphans) Option {
	return func(o *Observer) {
		o.orphans = orphans
	}
}

// WithNotifier has notifier told of a resource whose state changed, once
// that is written down, one that went missing among them, and of one whose
// record went, once it is gone: the roots whose graphs it is in are carried
// out at once, making it again, or making what waited on it, a container in a
// VM that came up, rather than at their next resync. What its node says again
// as it said it before tells nothing.
func WithNotifier(notifier graph.Notifier) Option {
	return func(o *Observer) {
		o.notifier = notifier
	}
}

// WithRefresh has what a node keeps saying of a resource written down again
// at least every so often, in place of every refreshAfter: often enough that
// whoever takes a resource to be gone when it was not heard of for a while,
// the reconcile loop, never takes one its node keeps saying to be.
func WithRefresh(every time.Duration) Option {
	return func(o *Observer) {
		o.refresh = every
	}
}

func NewObserver(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, logger *slog.Logger, options ...Option) *Observer {
	o := &Observer{registry: registry, resources: resources, refresh: refreshAfter, logger: logger}

	for _, option := range options {
		option(o)
	}

	return o
}

// Heartbeat writes down what a node's heartbeat at a moment says of one
// instance it holds, of a kind whose state is its nodes' to say.
//
// The instance of a resource that node holds is what its status is taken
// from, as its kind's machine says, unless the heartbeat is older than what
// was last heard of it, or than the command it is in flight on: what a node
// saw before a command reached it, or before the parent it lives in was
// restored, says nothing of it. A resource whose record says it is on another
// node is that node's to speak for.
//
// An instance nobody keeps a record of, of a kind that lives in nothing, is
// an orphan, which orphans are told of, when the observer was given them: a
// VM deleted while its node could not be told. They are told of one a record
// is found for too, which no orphan is. What lives in a parent is its
// parent's, and is never an orphan of its own. A kind whose strategy is a
// kind.Witness is then told of the instance, and whether a record is it: a
// container a VM's terminal made is the kind's to show, never the framework's
// to keep.
//
// A heartbeat says nothing of what its node does not hold: a resource its
// node goes on beating without a word of is the reconcile loop's to find, and
// to take to be gone through Unheard. A kind the control plane does not run,
// or whose state is not its nodes' to say, is not listened to.
//
// Nothing here fails the heartbeat: what could not be written down is
// reported, and the next beat says it all again; but for what a witness could
// not take in of the first look into a restored parent, which no later beat
// says again, and which it asks to be heard again for (kind.ErrHearAgain).
func (o *Observer) Heartbeat(ctx context.Context, nodeName string, at time.Time, instance kind.Observation) error {
	binding, registered := o.registry.Lookup(instance.Kind)
	if !registered {
		o.logger.DebugContext(ctx, "a node reported a kind the control plane does not run", "node", nodeName, "kind", instance.Kind)

		return nil
	}

	d := binding.Descriptor()
	if d.StateBy != kind.OnNode {
		return nil
	}

	kept := false

	if len(instance.UUID) > 0 {
		r, err := o.resources.GetOne(ctx, d.Name, instance.UUID)

		switch {
		case err == nil:
			kept = true

			// one whose record says it is on another node is that node's to
			// speak for.
			if r.Metadata.Node == nodeName {
				if err := o.take(ctx, d, nodeName, r, instance.Status, at, found); err != nil {
					o.logger.WarnContext(ctx, "could not write down what a node said of a resource", "error", err, "node", nodeName, "kind", d.Name, "uuid", instance.UUID)
				}
			}
		case !errors.Is(err, domain.ErrNotExists):
			o.logger.ErrorContext(ctx, "could not read what a node said it holds", "error", err, "node", nodeName, "kind", d.Name, "uuid", instance.UUID)

			return nil
		}
	}

	if o.orphans != nil && len(d.Parent) == 0 && len(instance.UUID) > 0 {
		if kept {
			o.orphans.Kept(ctx, d, nodeName, instance.UUID)
		} else if err := o.orphans.Orphaned(ctx, d, nodeName, instance.UUID, at); err != nil {
			o.logger.WarnContext(ctx, "could not tell what a node holds that nobody keeps a record of", "error", err, "node", nodeName, "kind", d.Name, "uuid", instance.UUID)
		}
	}

	if witness, witnesses := binding.Witness(); witnesses {
		if err := witness.Witnessed(ctx, nodeName, instance, kept, at); err != nil {
			o.logger.WarnContext(ctx, "a kind could not take in what a node reported of it", "error", err, "node", nodeName, "kind", d.Name)

			if errors.Is(err, kind.ErrHearAgain) {
				return err
			}
		}
	}

	return nil
}

// Unheard writes down, at a moment, what its node's silence says of a
// resource of a kind whose state is its nodes' to say: the node went on
// beating, up to then, without a word of it for long enough that it is not
// to be heard of. Which is long enough is the reconcile loop's to say.
//
// A resource that lives inside a parent is judged by the parent, the same
// way for every kind with one:
//
//   - inside a parent that is down, which its node cannot look inside, it
//     waits on the parent, as parents say the parent is, and is never taken
//     to be missing. With nobody to say what parents are doing, a parent is
//     taken to be up;
//   - one whose parent was restored since it was last heard of is not on the
//     disk the parent was given back, and is not made again but forgotten
//     (kind.CascadeReset), once the restored parent was looked into since it
//     was reset for that restore (Parents.Looked), and its node went on as
//     long without a word of it since that look: its silence counts from the
//     look, which decides what of it the restored disk holds, found there and
//     kept, or not; before the look, its silence says nothing, however long
//     the parent's dockerd takes to come up, and silence from before the
//     restore says nothing of it either way;
//   - and anything else is missing from its node, inside its parent or
//     inside none, as its kind's machine takes that.
//
// It is taken as anything its node says of it is, by the same guards: a
// moment older than what was last heard of it, or than the command it is in
// flight on, says nothing. Nor does one no later than a word of it heard in
// the meantime, which the silence it stands for was broken by.
func (o *Observer) Unheard(ctx context.Context, d kind.Descriptor, r resource.Record, at time.Time) error {
	var parent kind.Reference
	if len(d.Parent) > 0 {
		parent, _ = r.Metadata.Owner(d.Parent)
	}

	if len(parent.UUID) > 0 && o.parents != nil {
		down, err := o.parents.Down(ctx, parent)
		if err != nil {
			return err
		}

		if len(down) > 0 {
			return o.take(ctx, d, r.Metadata.Node, r, waitingOn(parent, down), at, unsaid)
		}
	}

	if r.Reset {
		// silence from before its parent was restored says nothing of what
		// the restored parent holds.
		recorded, err := r.Common()
		if err != nil || !at.After(recorded.ObservedAt) {
			return err
		}

		// nor does silence from before the restored parent was looked into
		// since, which counts from that look.
		if len(parent.UUID) > 0 && o.parents != nil {
			looked, says, err := o.parents.Looked(ctx, parent)

			switch {
			case err != nil:
				return err
			case !says:
			case looked.Before(r.ResetAt):
				return nil
			case recorded.ObservedAt.Before(looked):
				return o.silentSince(ctx, r, looked)
			}
		}

		o.logger.InfoContext(ctx, "forgetting a resource its restored parent does not have", "kind", d.Name, "uuid", r.Metadata.UUID, "parent", parent.UUID)

		if err := o.resources.Delete(ctx, d.Name, r.Metadata.UUID); err != nil {
			return err
		}

		o.changed(ctx, r)

		return nil
	}

	return o.take(ctx, d, r.Metadata.Node, r, missingIn(parent), at, unsaid)
}

// silentSince has the record r, of a resource its parent's restore reset,
// taken to have been heard of as of the first look into the restored parent
// since, looked: what the look found of it is known by then, and its silence
// counts from the look rather than from the restore, which the reconcile loop
// goes by. One written in the meantime is judged again at the loop's next
// pass, and one that is gone is let be.
func (o *Observer) silentSince(ctx context.Context, r resource.Record, looked time.Time) error {
	common, err := r.Common()
	if err != nil {
		return err
	}

	common.ObservedAt = looked

	if err := r.SetCommon(common); err != nil {
		return err
	}

	_, err = o.resources.Update(ctx, r)
	if errors.Is(err, resource.ErrConflict) || errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

// Take writes down what was observed at a moment of one resource its node
// holds, reading the resource again when something else wrote it first. A
// resource that moved to another node since is that node's to speak for.
func (o *Observer) Take(ctx context.Context, d kind.Descriptor, nodeName string, r resource.Record, status json.RawMessage, at time.Time) error {
	return o.take(ctx, d, nodeName, r, status, at, said)
}

// source is where what is taken onto a record comes from.
type source int

const (
	// said is what its node said of it.
	said source = iota

	// found is what its node said of it where it lives: one that was to be
	// reset to what its parent holds is then as it was found.
	found

	// unsaid is what its node's silence about it says, which a word of it
	// heard at the same moment, or later, breaks.
	unsaid
)

// take is Take, of what came from wherever from says.
func (o *Observer) take(ctx context.Context, d kind.Descriptor, nodeName string, r resource.Record, status json.RawMessage, at time.Time, from source) error {
	for try := 1; ; try++ {
		recorded, err := r.Common()
		if err != nil {
			return err
		}

		// older than what was last heard of it, or than what it was last
		// asked: what a node saw before a command reached it says nothing
		// about where the command is taking it. Silence is broken by a word
		// as late as itself.
		if at.Before(recorded.ObservedAt) || (from == unsaid && !at.After(recorded.ObservedAt)) || (d.Machine.IsInFlight(recorded.State) && at.Before(recorded.Since)) {
			return nil
		}

		change, err := Observe(d, &r, status, at)
		if err != nil {
			return err
		}

		if change.Gone {
			if err := o.resources.Delete(ctx, d.Name, r.Metadata.UUID); err != nil {
				return err
			}

			o.changed(ctx, r)

			return nil
		}

		observed, err := r.Common()
		if err != nil {
			return err
		}

		// gone from its node, what needs it makes it again; come up, what
		// waited on it is made: whatever its state changed to, the roots
		// that need it are carried out.
		moved := observed.State != recorded.State

		if from == found && r.Reset {
			r.Reset = false
			change.Changed = true
		}

		if !change.Changed && at.Sub(recorded.ObservedAt) < o.refresh {
			return nil
		}

		if change.Changed {
			r.Metadata.UpdatedAt = at
		}

		_, err = o.resources.Update(ctx, r)
		switch {
		case err == nil:
			if moved {
				o.changed(ctx, r)
			}

			return nil
		case errors.Is(err, domain.ErrNotExists):
			return nil
		case !errors.Is(err, resource.ErrConflict) || try >= tries:
			return err
		}

		r, err = o.resources.GetOne(ctx, d.Name, r.Metadata.UUID)
		if errors.Is(err, domain.ErrNotExists) {
			return nil
		} else if err != nil {
			return err
		}

		if r.Metadata.Node != nodeName {
			return nil
		}
	}
}

// changed tells the notifier, when there is one, that r changed: its state
// did, or its record is gone. What cannot be told is reported, and caught up
// with by the roots' next resync.
func (o *Observer) changed(ctx context.Context, r resource.Record) {
	if o.notifier == nil {
		return
	}

	if err := o.notifier.Changed(ctx, graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID}); err != nil {
		o.logger.WarnContext(ctx, "could not tell the graphs a resource is in that it changed", "error", err, "kind", r.Kind, "uuid", r.Metadata.UUID)
	}
}

// missingIn is what is observed of a resource its node has gone on without
// a word of, inside parent, or inside none: its node no longer holds it.
func missingIn(parent kind.Reference) json.RawMessage {
	observed := kind.Status{State: kind.Missing}
	if len(parent.UUID) > 0 {
		observed.Reason = fmt.Sprintf("its %s has none of it", parent.Kind)
	}

	return statusOf(observed)
}

// waitingOn is what is observed of a resource inside a parent its node
// cannot look into, because the parent is down.
func waitingOn(parent kind.Reference, down kind.State) json.RawMessage {
	return statusOf(kind.Status{State: kind.Waiting, Reason: fmt.Sprintf("its %s is %s", parent.Kind, down)})
}

func statusOf(observed kind.Status) json.RawMessage {
	status, err := json.Marshal(observed)
	if err != nil {
		panic(err)
	}

	return status
}
