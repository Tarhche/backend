// Package dispatch asks the resources of every kind for their kinds'
// commands: for a person, through the API, and for the control plane itself
// when it reconciles.
//
// Asking for a command moves a resource the way its kind's machine says,
// and makes what the command desires what the resource is expected to be. A
// command run in the control plane is carried out there and then, on the
// record. One run on a node is written down first, as the command the
// resource is waiting on, and sent second, on workloadCommand, to the node
// that holds the resource, carrying the resource as it was written down, so
// that the node needs no database. Always in that order: a command its node
// never hears of is sent again by the reconcile loop, which reads what was
// written, while a node that heard of something nobody wrote down is one
// nothing would ever correct.
package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// pollEvery is how often a wait for a command's result looks for it on
	// the resource, in case another control plane heard it.
	pollEvery = 500 * time.Millisecond

	// orphanEvery is how often a node is asked again to delete what nobody
	// keeps a record of, while it still holds it: a delete takes a moment, and
	// every heartbeat in the meantime says it again.
	orphanEvery = time.Minute

	// maxTries is how many tries of one command a resource remembers: a
	// result for one sent before them is too late to be its answer.
	maxTries = 16

	// deleteAction is the action every kind has that deletes its resources.
	deleteAction = "delete"
)

// Dispatcher asks resources for their kinds' commands.
type Dispatcher struct {
	resources resource.Repository
	producer  domain.Producer
	waiters   *waiters.Waiters
	now       func() time.Time
	poll      time.Duration

	// orphans are when each node was last asked to delete what it holds and
	// nobody keeps a record of, by node and uuid.
	lock    sync.Mutex
	orphans map[string]time.Time
}

// Option changes how a dispatcher goes about it.
type Option func(*Dispatcher)

// PollEvery is how often a wait for a command's result looks for it on the
// resource.
func PollEvery(interval time.Duration) Option {
	return func(d *Dispatcher) {
		d.poll = interval
	}
}

// New is a dispatcher that keeps resources in resources, sends their
// commands with producer, and lets waiters wait for what came of them. A
// clock of nil is the time now.
func New(resources resource.Repository, producer domain.Producer, waiting *waiters.Waiters, now func() time.Time, options ...Option) *Dispatcher {
	if now == nil {
		now = time.Now
	}

	d := &Dispatcher{resources: resources, producer: producer, waiters: waiting, now: now, poll: pollEvery, orphans: make(map[string]time.Time)}

	for _, option := range options {
		option(d)
	}

	return d
}

// Now is the time the dispatcher goes by.
func (d *Dispatcher) Now() time.Time {
	return d.now()
}

// Asked is what asking a resource for a command came to.
type Asked struct {
	// Record is the resource as it was left, and written down.
	Record resource.Record

	// Command is what its node is to be sent: written down as what the
	// resource is waiting on, and not sent yet. It is nil for a command
	// carried out in the control plane.
	Command *kind.Command

	// Gone says the resource's record was taken away: it was deleted in
	// place, or there was nothing of it anywhere to delete.
	Gone bool
}

// Ask asks a resource for one of its kind's commands, with its payload as it
// was given.
//
// The command has to be allowed in the state the resource is in; what is
// not is refused, as is a payload its codec refuses. A command for a node is
// readied by the kind first (kind.Preparer), which may refuse it, place the
// resource on a node, or give it what only the control plane knows. What it
// desires becomes what the resource is expected to be, and the resource moves
// as its machine says. A command run on a node is then written down and
// returned to be sent (Send), and one run in the control plane is carried out
// by the kind's strategy and its outcome written down.
//
// A command for a node, asked of a resource on none, has nowhere to go: a
// delete takes the record away, since there is nothing anywhere to delete;
// one that desires a state has that written down, as all that can be done
// for it until the resource is somewhere; and any other is
// kind.ErrUnreachable.
//
// Fresh says it is something new asked of the resource, by a person or by a
// lifetime ending, rather than the reconcile loop trying again for what is
// expected of it already: the tries at that start again from none.
func (d *Dispatcher) Ask(ctx context.Context, b kind.ControlPlaneBinding, r resource.Record, action string, payload json.RawMessage, fresh bool) (Asked, domain.ValidationErrors, error) {
	descriptor := b.Descriptor()

	a, found := descriptor.Action(action)
	if !found || a.Mode != kind.ModeCommand {
		return Asked{}, nil, fmt.Errorf("%w: a %s has no command %q", kind.ErrUnknownAction, descriptor.Name, action)
	}

	common, err := r.Common()
	if err != nil {
		return Asked{}, nil, err
	}

	if !descriptor.Allows(action, common.State) {
		return Asked{}, domain.ValidationErrors{"action": "invalid_state_transition"}, nil
	}

	// a node reads the payload again, but what is wrong with it is said here,
	// to whoever asked, and so is what the kind finds wrong with the command.
	if a.Runs == kind.OnNode {
		if _, invalid, err := a.Payload.Decode(payload); err != nil || len(invalid) > 0 {
			return Asked{}, invalid, err
		}

		if payload, err = Compact(payload); err != nil {
			return Asked{}, nil, err
		}

		prepared, invalid, err := b.Prepare(ctx, r.Raw, action, payload)
		if err != nil || len(invalid) > 0 {
			return Asked{}, invalid, err
		}

		// what it is, and which, is not the strategy's to change.
		prepared.Kind = r.Kind
		prepared.Metadata.UUID = r.Metadata.UUID
		r.Raw = prepared

		if common, err = r.Common(); err != nil {
			return Asked{}, nil, err
		}
	}

	now := d.now()

	if fresh {
		r.Attempts = 0
	}

	if a.Runs == kind.OnNode && len(r.Metadata.Node) == 0 {
		switch {
		case a.Desires == kind.Deleted:
			return d.forget(ctx, r)
		case len(a.Desires) > 0:
			return d.desired(ctx, r, common, a.Desires, now)
		}

		return Asked{}, nil, fmt.Errorf("%w: the %s %q is on no node yet", kind.ErrUnreachable, descriptor.Name, r.Metadata.UUID)
	}

	if len(a.Desires) > 0 {
		common.Expected = a.Desires
	}

	if to, moved := descriptor.Machine.Next(common.State, kind.OnAction(action)); moved && to != common.State {
		common.State = to
		common.Since = now
		common.Reason = ""
	}

	if err := r.SetCommon(common); err != nil {
		return Asked{}, nil, err
	}

	r.Metadata.UpdatedAt = now

	if a.Runs == kind.OnControlPlane {
		return d.apply(ctx, b, a, r, payload)
	}

	id, err := newID()
	if err != nil {
		return Asked{}, nil, err
	}

	attempt := r.Attempts

	r.Pending = &resource.Pending{Action: action, Payload: slices.Clone(payload), IDs: []string{id}, SentAt: now}
	r.Attempts++
	r.TriedAt = now

	written, err := d.resources.Update(ctx, r)
	if err != nil {
		return Asked{}, nil, err
	}

	return Asked{Record: written, Command: commandOf(written, id, action, attempt, payload)}, nil, nil
}

// apply carries out a command run in the control plane, and writes down
// what it left the resource as. One that deleted it takes its record away.
func (d *Dispatcher) apply(ctx context.Context, b kind.ControlPlaneBinding, a kind.Action, r resource.Record, payload json.RawMessage) (Asked, domain.ValidationErrors, error) {
	applied, invalid, err := b.Apply(ctx, r.Raw, a.Name, payload)
	if err != nil || len(invalid) > 0 {
		return Asked{}, invalid, err
	}

	// what it is, and which, is not the strategy's to change.
	applied.Kind = r.Kind
	applied.Metadata.UUID = r.Metadata.UUID
	r.Raw = applied

	common, err := r.Common()
	if err != nil {
		return Asked{}, nil, err
	}

	if a.Desires == kind.Deleted || common.State == kind.Deleted {
		return d.forget(ctx, r)
	}

	if len(common.Expected) > 0 && common.State == common.Expected {
		r.Attempts = 0
	}

	written, err := d.resources.Update(ctx, r)
	if err != nil {
		return Asked{}, nil, err
	}

	return Asked{Record: written}, nil, nil
}

// desired writes down what a command asked of a resource on no node desires,
// which is all that can be done for it there: it stays where it is, and is
// asked for what it is expected to be once it is somewhere.
func (d *Dispatcher) desired(ctx context.Context, r resource.Record, common kind.Status, state kind.State, now time.Time) (Asked, domain.ValidationErrors, error) {
	common.Expected = state

	if err := r.SetCommon(common); err != nil {
		return Asked{}, nil, err
	}

	r.Metadata.UpdatedAt = now

	written, err := d.resources.Update(ctx, r)
	if err != nil {
		return Asked{}, nil, err
	}

	return Asked{Record: written}, nil, nil
}

// Follow asks a resource for what its kind asks for of it as it is now: what
// its Reconcile says, in order, until one of them is a command for its node,
// which is written down and returned to be sent. One is in flight at a time,
// so what the kind asks for after it is asked for once it arrives, by the
// reconcile loop. It is what a resource just admitted, or just changed in the
// control plane, is asked at once, rather than on the loop's next pass.
//
// What the kind asks for and the state does not allow, or refuses, is left
// for the loop, which asks the kind again; the record is as it was asked so
// far either way.
func (d *Dispatcher) Follow(ctx context.Context, b kind.ControlPlaneBinding, r resource.Record) (Asked, error) {
	asked := Asked{Record: r}

	intents, err := b.Reconcile(ctx, r.Raw)
	if err != nil {
		return asked, err
	}

	for _, intent := range intents {
		common, err := asked.Record.Common()
		if err != nil {
			return asked, err
		}

		if !b.Descriptor().Allows(intent.Action, common.State) {
			return asked, nil
		}

		payload, err := PayloadOf(intent)
		if err != nil {
			return asked, err
		}

		next, invalid, err := d.Ask(ctx, b, asked.Record, intent.Action, payload, false)
		if err != nil {
			return asked, err
		}

		if len(invalid) > 0 {
			return asked, nil
		}

		asked = next

		if asked.Gone || asked.Command != nil {
			return asked, nil
		}
	}

	return asked, nil
}

// PayloadOf is what an intent's action is asked with, as it travels.
func PayloadOf(intent kind.Intent) (json.RawMessage, error) {
	if intent.Payload == nil {
		return nil, nil
	}

	return json.Marshal(intent.Payload)
}

// Again writes down that the command a resource is waiting on is sent once
// more, as its next try under an ID of its own, and returns it to be sent.
// What the resource was asked moves nothing again: it is on its way already.
func (d *Dispatcher) Again(ctx context.Context, r resource.Record) (Asked, error) {
	if r.Pending == nil {
		return Asked{}, fmt.Errorf("the %s %q is waiting on no command to send again", r.Kind, r.Metadata.UUID)
	}

	id, err := newID()
	if err != nil {
		return Asked{}, err
	}

	now := d.now()
	attempt := r.Attempts

	pending := *r.Pending
	pending.IDs = append(slices.Clone(pending.IDs), id)
	if len(pending.IDs) > maxTries {
		pending.IDs = pending.IDs[len(pending.IDs)-maxTries:]
	}

	pending.SentAt = now

	r.Pending = &pending
	r.Attempts++
	r.TriedAt = now
	r.Metadata.UpdatedAt = now

	written, err := d.resources.Update(ctx, r)
	if err != nil {
		return Asked{}, err
	}

	return Asked{Record: written, Command: commandOf(written, id, pending.Action, attempt, pending.Payload)}, nil
}

// Desire writes down what a resource is expected to be, without asking it
// for anything yet: what the reconcile loop asks for once it can be.
func (d *Dispatcher) Desire(ctx context.Context, r resource.Record, state kind.State, fresh bool) (resource.Record, error) {
	common, err := r.Common()
	if err != nil {
		return resource.Record{}, err
	}

	common.Expected = state

	if err := r.SetCommon(common); err != nil {
		return resource.Record{}, err
	}

	if fresh {
		r.Attempts = 0
	}

	r.Metadata.UpdatedAt = d.now()

	return d.resources.Update(ctx, r)
}

// Orphaned asks the node holding a resource nobody keeps a record of to
// delete it, as the delete of a resource of its kind that carries nothing but
// what it is and where: a VM deleted while its node could not be told, or
// made again by a command carried out after its delete was. Nobody is going
// to want it. Its node is asked at most once a minute for one resource,
// however often it reports it, and what came of it is heard as nobody's.
func (d *Dispatcher) Orphaned(ctx context.Context, desc kind.Descriptor, nodeName string, uuid string) error {
	if !d.orphan(nodeName, uuid) {
		return nil
	}

	id, err := newID()
	if err != nil {
		return err
	}

	_, err = d.Send(ctx, kind.Command{
		ID:     id,
		Kind:   desc.Name,
		UUID:   uuid,
		Action: deleteAction,
		Node:   nodeName,
		Resource: kind.Raw{
			Kind:     desc.Name,
			Metadata: kind.Metadata{UUID: uuid, Node: nodeName},
		},
	}, 0)

	return err
}

// orphan reports whether a node is to be asked to delete an orphan now, and
// writes down that it is: once a minute at most, for one resource. What was
// asked longer ago than that is let go of.
func (d *Dispatcher) orphan(nodeName string, uuid string) bool {
	d.lock.Lock()
	defer d.lock.Unlock()

	now := d.now()

	for asked, at := range d.orphans {
		if now.Sub(at) >= orphanEvery {
			delete(d.orphans, asked)
		}
	}

	key := nodeName + "/" + uuid
	if _, asked := d.orphans[key]; asked {
		return false
	}

	d.orphans[key] = now

	return true
}

// Forget takes a resource's record away, when there is nothing of it left
// anywhere to wait for.
func (d *Dispatcher) Forget(ctx context.Context, r resource.Record) error {
	return d.resources.Delete(ctx, r.Kind, r.Metadata.UUID)
}

func (d *Dispatcher) forget(ctx context.Context, r resource.Record) (Asked, domain.ValidationErrors, error) {
	if err := d.Forget(ctx, r); err != nil {
		return Asked{}, nil, err
	}

	return Asked{Record: r, Gone: true}, nil, nil
}

// Send sends a command to the node it is addressed to, on workloadCommand.
// What it asks has been written down by now, so a caller that has gone away
// does not take the command back with it.
//
// When wait is more than nothing, it waits for what came of the command for
// that long, and is that, or nil when nothing came in time: the command is
// still the resource's, and its result is taken whenever it comes.
func (d *Dispatcher) Send(ctx context.Context, command kind.Command, wait time.Duration) (*kind.Result, error) {
	var expected *waiters.Wait
	if wait > 0 && d.waiters != nil {
		expected = d.waiters.Expect(command.ID)
		defer expected.Done()
	}

	payload, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}

	if err := d.producer.Produce(context.WithoutCancel(ctx), kind.CommandName, payload); err != nil {
		return nil, err
	}

	if expected == nil {
		return nil, nil
	}

	result, answered := expected.For(ctx, wait, d.poll, d.answered(command))
	if !answered {
		return nil, nil
	}

	return &result, nil
}

// Delivered is what asking for a command came to, once it was sent and,
// when it was waited for, answered.
type Delivered struct {
	// Resource is the resource as it is now, unless it is Gone.
	Resource kind.Raw
	Gone     bool

	// Command is what its node was sent, for a command run on one, and
	// Result what came of it, when it was waited for and came in time.
	Command *kind.Command
	Result  *kind.Result
}

// Deliver sends the command asking came to, when it is one for a node, and
// waits for what came of it for as long as wait: it is then the resource as
// that left it.
func (d *Dispatcher) Deliver(ctx context.Context, asked Asked, wait time.Duration) (Delivered, error) {
	delivered := Delivered{Resource: asked.Record.Raw, Gone: asked.Gone, Command: asked.Command}

	if asked.Gone {
		delivered.Resource = kind.Raw{}
	}

	if asked.Command == nil {
		return delivered, nil
	}

	result, err := d.Send(ctx, *asked.Command, wait)
	if err != nil || result == nil {
		return delivered, err
	}

	delivered.Result = result

	latest, err := d.resources.GetOne(ctx, asked.Command.Kind, asked.Command.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		delivered.Resource = kind.Raw{}
		delivered.Gone = true
	case err != nil:
		return Delivered{}, err
	default:
		delivered.Resource = latest.Raw
	}

	return delivered, nil
}

// answered looks for what came of a command on its resource, which keeps
// its last command's answer: what another control plane heard is found there.
// A resource that is gone was deleted, which is what came of a delete; any
// other command is overtaken by it.
func (d *Dispatcher) answered(command kind.Command) waiters.Check {
	return func(ctx context.Context) (kind.Result, bool) {
		r, err := d.resources.GetOne(ctx, command.Kind, command.UUID)

		switch {
		case errors.Is(err, domain.ErrNotExists):
			result := kind.Result{ID: command.ID, Kind: command.Kind, UUID: command.UUID, Action: command.Action, Node: command.Node, OK: command.Action == deleteAction}
			if !result.OK {
				result.Reason = fmt.Sprintf("the %s is gone", command.Kind)
			}

			return result, true
		case err != nil:
			return kind.Result{}, false
		case r.Answer != nil && r.Answer.ID == command.ID:
			return *r.Answer, true
		}

		return kind.Result{}, false
	}
}

// commandOf is the command a resource, as it was written down, is sent.
func commandOf(r resource.Record, id string, action string, attempt int, payload json.RawMessage) *kind.Command {
	return &kind.Command{
		ID:       id,
		Kind:     r.Kind,
		UUID:     r.Metadata.UUID,
		Action:   action,
		Node:     r.Metadata.Node,
		Attempt:  attempt,
		Payload:  slices.Clone(payload),
		Resource: r.Raw,
	}
}

// Compact is a payload as it travels: nothing for nothing at all, and JSON
// without the spaces it was written with otherwise.
func Compact(payload json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var compacted bytes.Buffer
	if err := json.Compact(&compacted, trimmed); err != nil {
		return nil, fmt.Errorf("%w: %w", kind.ErrInvalidPayload, err)
	}

	return compacted.Bytes(), nil
}

// newID tells a command from every other, a try of the same one included.
func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	return id.String(), nil
}
