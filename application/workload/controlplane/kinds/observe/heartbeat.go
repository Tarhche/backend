package observe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// tries is how many times a record is read and written again when
	// something else wrote it in the meantime.
	tries = 3

	// refreshAfter is how long a resource whose node keeps saying the same
	// thing goes before that is written down again: often enough that when it
	// was last observed stays roughly true, rarely enough that a heartbeat is
	// not a write for every resource every second.
	refreshAfter = 15 * time.Second
)

// Parents say what the resources others live in are doing, when what lives
// in them cannot be looked at because of it: a Docker VM that is stopped,
// whose stacks and containers nobody can read until it runs again.
type Parents interface {
	// Down is the state parent is in, in its kind's own word, when what lives
	// in it cannot be looked at because of that state; and nothing when it
	// can be, or when that is not known, which concludes nothing about what
	// lives in it.
	Down(ctx context.Context, parent kind.Reference) (kind.State, error)
}

// Orphans are told of what a node holds that nobody keeps a record of: a VM
// deleted while its node could not be told, or made again by a command
// carried out after its delete was. Nobody is going to want it.
type Orphans interface {
	// Orphaned is told that nodeName holds a resource of d's kind, by uuid,
	// that has no record.
	Orphaned(ctx context.Context, d kind.Descriptor, nodeName string, uuid string) error
}

// Observer writes down what the nodes' heartbeats say of the resources of
// every kind the control plane runs.
type Observer struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	parents   Parents
	orphans   Orphans
	logger    *slog.Logger
}

// Option changes how an observer goes about it.
type Option func(*Observer)

// WithParents has what lives inside a parent its node did not look into
// observed waiting on it, as parents say the parent is.
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

func NewObserver(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, logger *slog.Logger, options ...Option) *Observer {
	o := &Observer{registry: registry, resources: resources, logger: logger}

	for _, option := range options {
		option(o)
	}

	return o
}

// Heartbeat writes down what a node's heartbeat of one kind at a moment says
// of the resources of that kind the node holds.
//
// A kind's report lists everything of it the node holds, so a resource of it
// that the node is said to hold and that the report leaves out is gone from
// the node, and observed missing. A resource that lives inside a parent is
// judged by the parent, the same way for every kind with one:
//
//   - inside a parent the node read, what the report leaves out is missing;
//     and one whose parent was restored since it was last seen there is not
//     made again but forgotten, since it is not on the disk the parent was
//     given back (kind.CascadeReset);
//   - inside a parent the node could not read, nothing is concluded about it
//     either way;
//   - and inside a parent the node did not look into at all, it waits on the
//     parent, as what the parent is doing says.
//
// What the node holds of a kind that lives in nothing, and that nobody keeps
// a record of, is an orphan, which orphans are told of, when the observer
// was given them. What lives in a parent is its parent's: it goes with it, or
// is reset to what it holds, and is never an orphan of its own.
//
// A kind whose strategy is a kind.Witness is then told the whole report, what
// nobody keeps a record of among it: a container a VM's terminal made is the
// kind's to show, never the framework's to keep.
//
// A kind that sent no heartbeat could not look at all this beat, and nothing
// is concluded from its silence. A kind the control plane does not run, or
// whose state is not its nodes' to say, is not listened to.
//
// The heartbeats of one beat's kinds are heard in no particular order, so a
// parent is what its own kind last said it is, which may be a beat behind.
// What lives in it is concluded from that only to wait on it, which the next
// look inside it undoes: what is missing is only ever its own kind's report's
// to say.
//
// Nothing here fails the heartbeat: what could not be written down is
// reported, and the next beat says it all again.
func (o *Observer) Heartbeat(ctx context.Context, nodeName string, kindName string, at time.Time, report kind.Report[json.RawMessage]) {
	binding, registered := o.registry.Lookup(kindName)
	if !registered {
		o.logger.DebugContext(ctx, "a node reported a kind the control plane does not run", "node", nodeName, "kind", kindName)

		return
	}

	d := binding.Descriptor()
	if d.StateBy != kind.OnNode {
		return
	}

	held, _, err := o.resources.GetAll(ctx, d.Name, resource.Filter{Node: nodeName}, 0, 0)
	if err != nil {
		o.logger.ErrorContext(ctx, "could not read what a node holds", "error", err, "node", nodeName, "kind", d.Name)

		return
	}

	for i := range held {
		if err := o.observe(ctx, d, nodeName, held[i], report, at); err != nil {
			o.logger.WarnContext(ctx, "could not write down what a node said of a resource", "error", err, "node", nodeName, "kind", d.Name, "uuid", held[i].Metadata.UUID)
		}
	}

	if err := o.orphaned(ctx, d, nodeName, held, report); err != nil {
		o.logger.WarnContext(ctx, "could not tell what a node holds that nobody keeps a record of", "error", err, "node", nodeName, "kind", d.Name)
	}

	if witness, witnesses := binding.Witness(); witnesses {
		if err := witness.Witnessed(ctx, nodeName, report, at); err != nil {
			o.logger.WarnContext(ctx, "a kind could not take in what a node reported of it", "error", err, "node", nodeName, "kind", d.Name)
		}
	}
}

// orphaned tells orphans of the resources a report lists that have no
// record, of a kind that lives in nothing. One held by the node is not; nor
// is one whose record says it is elsewhere, which is its own node's to speak
// for.
func (o *Observer) orphaned(ctx context.Context, d kind.Descriptor, nodeName string, held []resource.Record, report kind.Report[json.RawMessage]) error {
	if o.orphans == nil || len(d.Parent) > 0 {
		return nil
	}

	kept := make(map[string]bool, len(held))
	for i := range held {
		kept[held[i].Metadata.UUID] = true
	}

	for _, instance := range report.Instances {
		if len(instance.UUID) == 0 || kept[instance.UUID] {
			continue
		}

		_, err := o.resources.GetOne(ctx, d.Name, instance.UUID)
		switch {
		case errors.Is(err, domain.ErrNotExists):
			if err := o.orphans.Orphaned(ctx, d, nodeName, instance.UUID); err != nil {
				return err
			}
		case err != nil:
			return err
		}
	}

	return nil
}

// observe writes down what a report says of one resource its node holds, if
// it says anything.
func (o *Observer) observe(ctx context.Context, d kind.Descriptor, nodeName string, r resource.Record, report kind.Report[json.RawMessage], at time.Time) error {
	if instance, listed := report.Find(r.Metadata.UUID); listed {
		return o.take(ctx, d, nodeName, r, instance.Status, at, true)
	}

	var parent kind.Reference
	if len(d.Parent) > 0 {
		parent, _ = r.Metadata.Owner(d.Parent)
	}

	switch {
	case report.Missing(r.Metadata.UUID, parent.UUID) && r.Reset:
		// a look taken before its parent was restored says nothing of what
		// the restored parent holds.
		if recorded, err := r.Common(); err != nil || at.Before(recorded.ObservedAt) {
			return err
		}

		o.logger.InfoContext(ctx, "forgetting a resource its restored parent does not have", "kind", d.Name, "uuid", r.Metadata.UUID, "parent", parent.UUID)

		return o.resources.Delete(ctx, d.Name, r.Metadata.UUID)

	case report.Missing(r.Metadata.UUID, parent.UUID):
		return o.take(ctx, d, nodeName, r, missingIn(parent), at, false)

	case report.Unread(parent.UUID) && o.parents != nil:
		down, err := o.parents.Down(ctx, parent)
		if err != nil || len(down) == 0 {
			return err
		}

		return o.take(ctx, d, nodeName, r, waitingOn(parent, down), at, false)
	}

	return nil
}

// Take writes down what was observed at a moment of one resource its node
// holds, reading the resource again when something else wrote it first. A
// resource that moved to another node since is that node's to speak for.
func (o *Observer) Take(ctx context.Context, d kind.Descriptor, nodeName string, r resource.Record, status json.RawMessage, at time.Time) error {
	return o.take(ctx, d, nodeName, r, status, at, false)
}

// take is Take, of a resource its node found where it lives when found says
// so: one that was to be reset to what its parent holds is then as it was
// found.
func (o *Observer) take(ctx context.Context, d kind.Descriptor, nodeName string, r resource.Record, status json.RawMessage, at time.Time, found bool) error {
	for try := 1; ; try++ {
		recorded, err := r.Common()
		if err != nil {
			return err
		}

		// older than what was last heard of it, or than what it was last
		// asked: what a node saw before a command reached it says nothing
		// about where the command is taking it.
		if at.Before(recorded.ObservedAt) || (d.Machine.IsInFlight(recorded.State) && at.Before(recorded.Since)) {
			return nil
		}

		change, err := Observe(d, &r, status, at)
		if err != nil {
			return err
		}

		if change.Gone {
			return o.resources.Delete(ctx, d.Name, r.Metadata.UUID)
		}

		if found && r.Reset {
			r.Reset = false
			change.Changed = true
		}

		if !change.Changed && at.Sub(recorded.ObservedAt) < refreshAfter {
			return nil
		}

		if change.Changed {
			r.Metadata.UpdatedAt = at
		}

		_, err = o.resources.Update(ctx, r)
		switch {
		case err == nil, errors.Is(err, domain.ErrNotExists):
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

// missingIn is what a report says of a resource it could have listed and did
// not, inside parent, or inside none: its node no longer holds it.
func missingIn(parent kind.Reference) json.RawMessage {
	observed := kind.Status{State: kind.Missing}
	if len(parent.UUID) > 0 {
		observed.Reason = fmt.Sprintf("its %s has none of it", parent.Kind)
	}

	return statusOf(observed)
}

// waitingOn is what is observed of a resource inside a parent its node did
// not look into, because the parent is down.
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
