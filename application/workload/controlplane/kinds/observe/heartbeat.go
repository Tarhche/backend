package observe

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"slices"
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

// missing is what a report says of a resource it could have listed and did
// not: its node no longer holds it.
var missing = func() json.RawMessage {
	status, err := json.Marshal(kind.Status{State: kind.Missing})
	if err != nil {
		panic(err)
	}

	return status
}()

// Observer writes down what the nodes' heartbeats say of the resources of
// every kind the control plane runs.
type Observer struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	logger    *slog.Logger
}

func NewObserver(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, logger *slog.Logger) *Observer {
	return &Observer{registry: registry, resources: resources, logger: logger}
}

// Heartbeat writes down what a node's heartbeat at a moment says, kind by
// kind, of the resources the node holds.
//
// A kind's report lists everything of it the node holds, so a resource of it
// that the node is said to hold and that the report leaves out is gone from
// the node, and observed missing; but one inside a parent the report could
// not look into is not: nothing is concluded about it either way. A kind
// that sent no report could not look at all this beat, and nothing is
// concluded from its silence. A kind the control plane does not run, or
// whose state is not its nodes' to say, is not listened to.
//
// Nothing here fails the heartbeat: what could not be written down is
// reported, and the next beat says it all again.
func (o *Observer) Heartbeat(ctx context.Context, nodeName string, at time.Time, reports map[string]kind.Report[json.RawMessage]) {
	for _, kindName := range slices.Sorted(maps.Keys(reports)) {
		binding, registered := o.registry.Lookup(kindName)
		if !registered {
			o.logger.DebugContext(ctx, "a node reported a kind the control plane does not run", "node", nodeName, "kind", kindName)

			continue
		}

		d := binding.Descriptor()
		if d.StateBy != kind.OnNode {
			continue
		}

		held, _, err := o.resources.GetAll(ctx, d.Name, resource.Filter{Node: nodeName}, 0, 0)
		if err != nil {
			o.logger.ErrorContext(ctx, "could not read what a node holds", "error", err, "node", nodeName, "kind", d.Name)

			continue
		}

		report := reports[kindName]

		for i := range held {
			status, seen := observed(d, held[i], report)
			if !seen {
				continue
			}

			if err := o.Take(ctx, d, nodeName, held[i], status, at); err != nil {
				o.logger.WarnContext(ctx, "could not write down what a node said of a resource", "error", err, "node", nodeName, "kind", d.Name, "uuid", held[i].Metadata.UUID)
			}
		}
	}
}

// Take writes down what was observed at a moment of one resource its node
// holds, reading the resource again when something else wrote it first. A
// resource that moved to another node since is that node's to speak for.
func (o *Observer) Take(ctx context.Context, d kind.Descriptor, nodeName string, r resource.Record, status json.RawMessage, at time.Time) error {
	for try := 1; ; try++ {
		recorded, err := r.Common()
		if err != nil {
			return err
		}

		// older than what was last heard of it.
		if at.Before(recorded.ObservedAt) {
			return nil
		}

		change, err := Observe(d, &r, status, at)
		if err != nil {
			return err
		}

		if change.Gone {
			return o.resources.Delete(ctx, d.Name, r.Metadata.UUID)
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

// observed is what a report says of a resource, and whether it says
// anything: what it lists of it, or that it is missing when the report could
// see where it would be and does not list it.
func observed(d kind.Descriptor, r resource.Record, report kind.Report[json.RawMessage]) (json.RawMessage, bool) {
	if instance, listed := report.Find(r.Metadata.UUID); listed {
		return instance.Status, true
	}

	var parent kind.Reference
	if len(d.Parent) > 0 {
		parent, _ = r.Metadata.Owner(d.Parent)
	}

	if report.Missing(r.Metadata.UUID, parent.UUID) {
		return missing, true
	}

	return nil, false
}
