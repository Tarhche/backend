// Package resourceResult hears what came of the commands the nodes were
// sent, for every kind, on workloadResult.
//
// A result is taken onto its resource only when the resource is waiting on
// that command: a result for a command sent before something else was asked,
// or one heard already, is too late to say anything the record does not know
// better. Whoever waits for the command is told either way, since it is what
// came of their command.
//
// A command that restores a resource from a snapshot (kind.Action.Restores),
// carried out, resets what lives in the resource to what it now holds, as the
// kinds that live in it say, before the result is taken: told first, so that
// a result heard again tells it again rather than not at all.
//
// A result that will never be taken, one that cannot be read, of a kind not
// run here, for a resource that is gone, is not failed: redelivered, it would
// be refused the same way, at once and for ever. Only what may go another way
// next time fails it: the database, or a record written by something else
// every time it was read.
package resourceResult

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a record is read and written again when
// something else wrote it in the meantime.
const tries = 5

// Restorer resets what lives in a resource restored from a snapshot, at a
// moment, to what the resource holds afterwards, as the kinds that live in it
// say.
type Restorer interface {
	Restored(ctx context.Context, parent kind.Reference, at time.Time) error
}

// Result takes the results of commands onto their resources.
type Result struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	waiters   *waiters.Waiters
	restorer  Restorer
	logger    *slog.Logger
	now       func() time.Time
}

var _ domain.MessageHandler = &Result{}

// Option changes how results are taken.
type Option func(*Result)

// WithRestorer has what lives in a resource restored from a snapshot reset
// by restorer once the restore is carried out.
func WithRestorer(restorer Restorer) Option {
	return func(r *Result) {
		r.restorer = restorer
	}
}

// NewResult is a handler that keeps resources in resources and tells
// waiters what came of their commands. A clock of nil is the time now.
func NewResult(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, waiting *waiters.Waiters, logger *slog.Logger, now func() time.Time, options ...Option) *Result {
	if now == nil {
		now = time.Now
	}

	r := &Result{registry: registry, resources: resources, waiters: waiting, logger: logger, now: now}

	for _, option := range options {
		option(r)
	}

	return r
}

func (h *Result) Handle(ctx context.Context, data []byte) error {
	var result kind.Result
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
func (h *Result) take(ctx context.Context, result kind.Result) error {
	binding, registered := h.registry.Lookup(result.Kind)
	if !registered {
		return fmt.Errorf("%w: %w: %q", errNotTaken, kind.ErrUnknownKind, result.Kind)
	}

	d := binding.Descriptor()

	at := result.At
	if at.IsZero() {
		at = h.now()
	}

	for try := 1; ; try++ {
		r, err := h.resources.GetOne(ctx, d.Name, result.UUID)
		if errors.Is(err, domain.ErrNotExists) {
			return nil
		} else if err != nil {
			return err
		}

		change, err := observe.Answer(d, &r, result, at)
		switch {
		case errors.Is(err, observe.ErrNotWaitedOn):
			return nil
		case err != nil:
			return fmt.Errorf("%w: %w", errNotTaken, err)
		}

		if action, _ := d.Action(result.Action); result.OK && action.Restores && h.restorer != nil {
			if err := h.restorer.Restored(ctx, kind.Reference{Kind: d.Name, UUID: r.Metadata.UUID}, at); err != nil {
				return err
			}
		}

		if change.Gone {
			return h.resources.Delete(ctx, d.Name, r.Metadata.UUID)
		}

		r.Metadata.UpdatedAt = h.now()

		_, err = h.resources.Update(ctx, r)
		switch {
		case err == nil, errors.Is(err, domain.ErrNotExists):
			return nil
		case errors.Is(err, resource.ErrConflict) && try < tries:
			continue
		default:
			return err
		}
	}
}
