// Package recordResult hears what came of the commands the nodes were
// sent, for every kind, on workloadResourceActedOn.
//
// A result is taken onto its resource only when the resource is waiting on
// that command: a result for a command sent before something else was asked,
// or one heard already, is too late to say anything the record does not know
// better. Whoever waits for the command is told either way, since it is what
// came of their command.
//
// A command that restores a resource from a snapshot (kind.Action.Restores),
// carried out, resets what lives in the resource to what it now holds, as the
// kinds that live in it say, as of the restore's own moment, which its result
// says (kind.Restore), before the result is taken: told first, so that a
// result heard again tells it again rather than not at all. And it is taken
// whatever its resource was asked since, for as long as the resource's record
// says an earlier restore, by the record it was sent from alone, which keeps
// the restores sent to it until they are taken (resource.Record.Restores): a
// resource whose node went quiet while it was being restored, failed as lost,
// waits on nothing any more, but its disk was replaced all the same, and what
// lives in it, reset when the result was first heard, waits for its record to
// say so; while one made again under its uuid since, by a graph whose root
// its delete kept, was never restored, and takes nothing of it. Only what the
// result says of the restore is taken then, nothing of what was asked since
// undone: what the resource is doing is its node's to say next. Nor is what
// lives in one on its way out reset by a restore it waits on no more, which
// goes with it.
//
// A result that will never be taken, one that cannot be read, of a kind not
// run here, for a resource that is gone, is not failed: redelivered, it would
// be refused the same way, at once and for ever. Only what may go another way
// next time fails it: the database, or a record written by something else
// every time it was read.
//
// A delete carried out, once its resource's record is gone, a create that
// failed, and any result that changed its resource's state, once that is
// written down, are told to the graphs the resource is in (WithNotifier),
// whose roots are carried out at once: a VM that came up has what waited on
// it made then.
package recordResult

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// tries is how many times a record is read and written again when
	// something else wrote it in the meantime.
	tries = 5

	// createAction is the action that makes a resource, of every kind that
	// is made on a node.
	createAction = "create"
)

// Restorer resets what lives in a resource restored from a snapshot, as of
// the moment it was restored, to what the resource holds afterwards, as the
// kinds that live in it say.
type Restorer interface {
	Restored(ctx context.Context, parent kind.Reference, at time.Time) error
}

// ResourceActedOnHandler takes the results of commands onto their resources.
type ResourceActedOnHandler struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	waiters   *waiters.Waiters
	restorer  Restorer
	notifier  graph.Notifier
	logger    *slog.Logger
	now       func() time.Time
}

var _ domain.MessageHandler = &ResourceActedOnHandler{}

// Option changes how results are taken.
type Option func(*ResourceActedOnHandler)

// WithRestorer has what lives in a resource restored from a snapshot reset
// by restorer once the restore is carried out.
func WithRestorer(restorer Restorer) Option {
	return func(r *ResourceActedOnHandler) {
		r.restorer = restorer
	}
}

// WithNotifier has notifier told of a resource whose delete was carried out,
// once its record is gone, and of one whose create failed, or whose state a
// result changed, once that is written down: the roots whose graphs it is in
// are carried out at once, rather than at their next resync.
func WithNotifier(notifier graph.Notifier) Option {
	return func(r *ResourceActedOnHandler) {
		r.notifier = notifier
	}
}

// NewResourceActedOnHandler is a handler that keeps resources in resources
// and tells waiters what came of their commands. A clock of nil is the time
// now.
func NewResourceActedOnHandler(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, waiting *waiters.Waiters, logger *slog.Logger, now func() time.Time, options ...Option) *ResourceActedOnHandler {
	if now == nil {
		now = time.Now
	}

	r := &ResourceActedOnHandler{registry: registry, resources: resources, waiters: waiting, logger: logger, now: now}

	for _, option := range options {
		option(r)
	}

	return r
}

func (h *ResourceActedOnHandler) Handle(ctx context.Context, data []byte) error {
	var result kind.ResourceActedOn
	if err := json.Unmarshal(data, &result); err != nil {
		h.logger.ErrorContext(ctx, "a command's result that cannot be read", "error", err)

		return nil
	}

	if len(result.ID) == 0 || len(result.UUID) == 0 {
		h.logger.ErrorContext(ctx, "a command's result that names no command or no resource", "kind", result.Kind, "action", result.Action)

		return nil
	}

	if !result.OK {
		h.logger.WarnContext(ctx, "a command failed", "kind", result.Kind, "uuid", result.UUID, "action", result.Action, "node", result.Node, "attempt", result.Attempt, "reason", result.Reason)
	}

	err := h.take(ctx, result)
	if err != nil && !permanent(err) {
		// asked for again: what is waiting is told once it is taken.
		return err
	}

	if err != nil {
		h.logger.ErrorContext(ctx, "a command's result that cannot be taken", "error", err, "kind", result.Kind, "uuid", result.UUID, "action", result.Action)
	}

	if h.waiters != nil {
		h.waiters.Answer(result)
	}

	return nil
}

// errNotTaken wraps what will never let a result be taken.
var errNotTaken = errors.New("the result will never be taken")

func permanent(err error) bool {
	return errors.Is(err, errNotTaken)
}

// take takes a result onto its resource, reading the resource again when
// something else wrote it first.
func (h *ResourceActedOnHandler) take(ctx context.Context, result kind.ResourceActedOn) error {
	binding, registered := h.registry.Lookup(result.Kind)
	if !registered {
		return fmt.Errorf("%w: %w: %q", errNotTaken, kind.ErrUnknownKind, result.Kind)
	}

	d := binding.Descriptor()

	at := result.At
	if at.IsZero() {
		at = h.now()
	}

	// a restore carried out says when, which what lives in its resource is
	// reset as of: when it says nothing of it, as of when it was heard of.
	action, _ := d.Action(result.Action)
	restores := result.OK && action.Restores

	var restored kind.Restore
	if restores {
		var err error
		if restored, err = resource.RestoreOf(result.Status); err != nil {
			return fmt.Errorf("%w: %w", errNotTaken, err)
		}
	}

	resetAt := at
	if !restored.RestoredAt.IsZero() {
		resetAt = restored.RestoredAt
	}

	for try := 1; ; try++ {
		r, err := h.resources.GetOne(ctx, d.Name, result.UUID)
		if errors.Is(err, domain.ErrNotExists) {
			return nil
		} else if err != nil {
			return err
		}

		before, err := r.Common()
		if err != nil {
			return fmt.Errorf("%w: %w", errNotTaken, err)
		}

		// a restore its resource waits on no more is taken all the same,
		// while its record says an earlier one, by the record it was sent
		// from alone: one made again under its uuid since never was.
		late := false

		change, err := observe.Answer(d, &r, result, at)
		switch {
		case errors.Is(err, observe.ErrNotWaitedOn) && restores && slices.Contains(r.Restores, result.ID):
			taken, err := took(&r, restored)
			switch {
			case err != nil:
				return fmt.Errorf("%w: %w", errNotTaken, err)
			case !taken:
				return nil
			}

			late, change = true, observe.Change{Changed: true}
		case errors.Is(err, observe.ErrNotWaitedOn):
			return nil
		case err != nil:
			return fmt.Errorf("%w: %w", errNotTaken, err)
		}

		after, err := r.Common()
		if err != nil {
			return fmt.Errorf("%w: %w", errNotTaken, err)
		}

		r.Restores = slices.DeleteFunc(slices.Clone(r.Restores), func(id string) bool { return id == result.ID })

		// what lives in one on its way out goes with it: its restore is
		// taken onto its record, and nothing in it is reset.
		if restores && h.restorer != nil && !(late && going(after, r)) {
			if err := h.restorer.Restored(ctx, kind.Reference{Kind: d.Name, UUID: r.Metadata.UUID}, resetAt); err != nil {
				return err
			}
		}

		if change.Gone {
			if err := h.resources.Delete(ctx, d.Name, r.Metadata.UUID); err != nil {
				return err
			}

			h.changed(ctx, r)

			return nil
		}

		r.Metadata.UpdatedAt = h.now()

		_, err = h.resources.Update(ctx, r)
		switch {
		case err == nil:
			if late || after.State != before.State || (!result.OK && result.Action == createAction) {
				h.changed(ctx, r)
			}

			return nil
		case errors.Is(err, domain.ErrNotExists):
			return nil
		case errors.Is(err, resource.ErrConflict) && try < tries:
			continue
		default:
			return err
		}
	}
}

// going reports whether the resource r is the record of, its status's
// common part common, is on its way out: asked to be deleted, or expected to
// be.
func going(common kind.Status, r resource.Record) bool {
	return common.Expected == kind.Deleted || common.State == kind.Deleted || !r.Metadata.DeletionRequestedAt.IsZero()
}

// took takes onto r, the record of a resource that waits on none of the
// commands a restore was sent as, what that restore's result says of it,
// restored, and reports whether it did: only while r says an earlier restore,
// or none, so that a result heard again, once it was taken, takes nothing, nor
// does one of a restore before the last one taken. A result that says nothing
// of when it was carried out is taken never.
func took(r *resource.Record, restored kind.Restore) (bool, error) {
	if restored.RestoredAt.IsZero() {
		return false, nil
	}

	recorded, err := resource.RestoreOf(r.Status)
	if err != nil {
		return false, err
	}

	if !recorded.RestoredAt.Before(restored.RestoredAt) {
		return false, nil
	}

	if r.Status, err = resource.WithRestore(r.Status, restored); err != nil {
		return false, err
	}

	return true, nil
}

// changed tells the notifier, when there is one, that r changed: its delete
// was carried out, its create failed, its state moved, or it took a restore
// it waited on no more. What cannot be told is reported, and caught up with
// by the roots' next resync.
func (h *ResourceActedOnHandler) changed(ctx context.Context, r resource.Record) {
	if h.notifier == nil {
		return
	}

	if err := h.notifier.Changed(ctx, graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID}); err != nil {
		h.logger.WarnContext(ctx, "could not tell the graphs a resource is in that it changed", "error", err, "kind", r.Kind, "uuid", r.Metadata.UUID)
	}
}
