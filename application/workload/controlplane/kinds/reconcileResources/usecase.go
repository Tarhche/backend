// Package reconcileResources brings the resources of every kind back to
// what was asked of them, one pass at a time, on the control plane's own
// heartbeat.
//
// What a resource is expected to be is written down; what it is doing is
// what its node last observed. A resource whose node fell silent, one whose
// lifetime is over, a command that was lost, a resource that fell over: each
// leaves the two disagreeing, and a pass looks at that disagreement and asks
// for whatever closes it. Most of that is the same for every kind, and is
// done here, in this order:
//
//   - a resource expected deleted, on a node that fell silent, is forgotten
//     once its delete was sent: the delete waits in the node's stream for it
//     to come back, and nothing is left to wait for here. One whose delete
//     was not sent yet is sent it, the one thing a silent node can be sent;
//   - one whose lifetime is over is deleted;
//   - one whose node fell silent is failed as node_lost, unless it ended
//     already: a stopped one stays stopped. Nothing is asked of it, since
//     nothing can be, and what it is expected to be stays as it was, so it is
//     brought back once its node is. Of a kind whose state the control plane
//     keeps, such as a snapshot, the node is only what a command was sent to:
//     its silence fails one waiting on that command, and nothing else;
//   - one in flight is waited on, until its command has been unanswered for
//     longer than it takes, its timeout (kind.Action.Timeout) and then the
//     loop's patience, and then the command is sent again, as another try.
//     One in flight with no command to wait on, which nothing will move on,
//     is failed;
//   - one whose parent was restored from a snapshot waits for the next look
//     inside the parent, which keeps it as it is found or forgets it
//     (kind.CascadeReset);
//   - one that has been tried for already, and is not yet what it is
//     expected to be, is left alone for longer each time: its backoff. The
//     tries are forgotten once it has been what it is expected to be for as
//     long as the last wait, so one that falls over again as soon as it is
//     brought back is brought back less and less often;
//   - one expected deleted is asked for its delete;
//   - and for everything else, the kind decides what to ask for (its
//     Reconcile), and what it asks for is asked.
//
// Nothing found wrong with one resource stops a pass: it is reported, and
// the next pass tries it again. Nothing here is a message, so nothing is
// redelivered for ever either.
package reconcileResources

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// ReasonNodeLost is why a resource whose node fell silent failed. It is
	// not given up on: a node that comes back reports what it holds.
	ReasonNodeLost = "node_lost"

	// ReasonStuck is why a resource that was in flight with nothing to wait
	// on failed.
	ReasonStuck = "nothing came of it"

	// deleteAction is the action every kind has that deletes its resources.
	deleteAction = "delete"

	// nodesLimit is the most nodes looked at. A workload has a handful.
	nodesLimit uint = 100
)

// Config is how patient a pass is.
type Config struct {
	// Batch is how many resources are read at a time. A pass works through
	// all of them, a batch at a time, so that how many there are decides how
	// long it takes rather than whether it covers them all.
	Batch uint

	// NodeSilentAfter is how long a node may go unheard before what it holds
	// is taken to be lost.
	NodeSilentAfter time.Duration

	// Patience is how long a command a resource in flight is waiting on goes
	// unanswered before it is sent again, beyond what its node may take over
	// it: a command given a timeout (kind.Action.Timeout) is waited on for
	// as long as Timeouts sizes it first, and one given none for nothing more.
	Patience time.Duration

	// Timeouts are how long a node may take over a command given each
	// timeout: an image pulled first, or a VM's disk streamed to or from the
	// bucket. They are what the nodes give such a command, so that it is not
	// sent again while its node is still carrying it out, holding one of the
	// node's command slots and its resource's lock for a second time.
	Timeouts map[kind.Timeout]time.Duration

	// Backoff is how long a resource that was tried for once is left before
	// it is tried for again; it doubles with every try after, up to
	// MaxBackoff.
	Backoff    time.Duration
	MaxBackoff time.Duration
}

// DefaultConfig is how patient the control plane is.
func DefaultConfig() Config {
	return Config{
		Batch:           20,
		NodeSilentAfter: 30 * time.Second,
		Patience:        5 * time.Minute,
		Backoff:         15 * time.Second,
		MaxBackoff:      15 * time.Minute,
	}
}

// waited is how long a command for action, of the kind d describes, goes
// unanswered before it is sent again: its timeout, and then the patience any
// command is given.
func (c Config) waited(d kind.Descriptor, action string) time.Duration {
	a, _ := d.Action(action)

	return c.Timeouts[a.Timeout] + c.Patience
}

// backoff is how long a resource tried for attempts times is left before it
// is tried for again.
func (c Config) backoff(attempts int) time.Duration {
	if attempts <= 0 {
		return 0
	}

	wait := c.Backoff
	for range attempts - 1 {
		if wait >= c.MaxBackoff {
			break
		}

		wait *= 2
	}

	return min(wait, c.MaxBackoff)
}

// UseCase is one pass over the resources of every kind.
type UseCase struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	nodes      node.Repository
	dispatcher *dispatch.Dispatcher
	logger     *slog.Logger
	config     Config
}

func NewUseCase(
	registry *kind.Registry[kind.ControlPlaneBinding],
	resources resource.Repository,
	nodes node.Repository,
	dispatcher *dispatch.Dispatcher,
	logger *slog.Logger,
	config Config,
) *UseCase {
	return &UseCase{
		registry:   registry,
		resources:  resources,
		nodes:      nodes,
		dispatcher: dispatcher,
		logger:     logger,
		config:     config,
	}
}

// Execute makes one pass over every kind registered. A pass with no kind
// registered does nothing at all.
//
// What is read moves while it is read, a resource deleted during a pass
// shifting the rest along, so one may be looked at twice, which asks for what
// it needs twice and is the same answer, or missed, which the next pass picks
// up.
func (uc *UseCase) Execute(ctx context.Context) error {
	bindings := uc.registry.All()
	if len(bindings) == 0 {
		return nil
	}

	// what they are judged against is when the pass began, so that one is
	// not called late for the time a long pass took to reach it.
	now := uc.dispatcher.Now()

	alive, err := uc.aliveNodes(ctx, now)
	if err != nil {
		return err
	}

	var failed error

	for _, binding := range bindings {
		failed = errors.Join(failed, uc.pass(ctx, binding, now, alive))
	}

	return failed
}

// pass looks at every resource of one kind.
func (uc *UseCase) pass(ctx context.Context, binding kind.ControlPlaneBinding, now time.Time, alive map[string]bool) error {
	d := binding.Descriptor()

	for offset := uint(0); ; offset += uc.config.Batch {
		records, total, err := uc.resources.GetAll(ctx, d.Name, resource.Filter{}, offset, uc.config.Batch)
		if err != nil {
			return err
		}

		for i := range records {
			if err := uc.look(ctx, binding, records[i], now, alive); err != nil {
				uc.logger.ErrorContext(ctx, "could not bring a resource back to what was asked of it", "error", err, "kind", d.Name, "uuid", records[i].Metadata.UUID)
			}
		}

		if len(records) == 0 || offset+uc.config.Batch >= total {
			return nil
		}
	}
}

// aliveNodes is which nodes have spoken lately.
func (uc *UseCase) aliveNodes(ctx context.Context, now time.Time) (map[string]bool, error) {
	nodes, err := uc.nodes.GetAll(ctx, 0, nodesLimit)
	if err != nil {
		return nil, err
	}

	alive := make(map[string]bool, len(nodes))
	for i := range nodes {
		alive[nodes[i].Name] = now.Sub(nodes[i].LastHeartbeatAt) <= uc.config.NodeSilentAfter
	}

	return alive, nil
}

// look asks for what one resource is missing, if it is missing anything.
func (uc *UseCase) look(ctx context.Context, binding kind.ControlPlaneBinding, r resource.Record, now time.Time, alive map[string]bool) error {
	d := binding.Descriptor()

	common, err := r.Common()
	if err != nil {
		return err
	}

	// a node speaks for what it holds, and, of a kind whose state is the
	// control plane's, only for the command it was sent and has not answered.
	silent := len(r.Metadata.Node) > 0 && !alive[r.Metadata.Node] && (d.StateBy == kind.OnNode || r.Pending != nil)

	switch {
	case common.State == kind.Deleted:
		return uc.dispatcher.Forget(ctx, r)

	// its delete was sent, and waits in the node's stream for the node to
	// come back: there is nothing left to wait for here.
	case silent && common.Expected == kind.Deleted && r.Pending != nil && r.Pending.Action == deleteAction:
		uc.logger.InfoContext(ctx, "forgetting a resource deleted on a node that has gone quiet", "kind", d.Name, "uuid", r.Metadata.UUID, "node", r.Metadata.Node)

		return uc.dispatcher.Forget(ctx, r)

	case common.Expected != kind.Deleted && r.Metadata.Expired(now):
		uc.logger.InfoContext(ctx, "deleting a resource that has outlived its lifetime", "kind", d.Name, "uuid", r.Metadata.UUID, "expires_at", r.Metadata.ExpiresAt)

		return uc.remove(ctx, binding, r, common, true)

	// nothing can be asked of a silent node but a delete, which waits in its
	// stream; one on its way somewhere is not getting there.
	case silent && (common.Expected != kind.Deleted || d.Machine.IsInFlight(common.State)):
		return uc.lost(ctx, d, r, common, now)

	case d.Machine.IsInFlight(common.State):
		return uc.inFlight(ctx, d, r, common, now)

	// its parent was restored from a snapshot: the next look inside the
	// parent keeps it as it is found there, or forgets it, and making it
	// again before that would make what the restored disk does not have.
	case r.Reset:
		return nil

	// what it is expected to be, for as long as it was last made to wait:
	// it stayed there, and the tries it took start over.
	case r.Attempts > 0 && len(common.Expected) > 0 && common.State == common.Expected && now.Sub(common.Since) >= uc.config.backoff(r.Attempts):
		return uc.settled(ctx, r)

	// tried for already, and not there yet, or not for long: given time
	// before the next try.
	case r.Attempts > 0 && now.Sub(r.TriedAt) < uc.config.backoff(r.Attempts):
		return nil

	case common.Expected == kind.Deleted:
		return uc.remove(ctx, binding, r, common, false)
	}

	intents, err := binding.Reconcile(ctx, r.Raw)
	if err != nil {
		return err
	}

	return uc.ask(ctx, binding, r, common, intents, false)
}

// settled forgets the tries it took to make a resource what it is expected
// to be, once it has stayed so: the next time it falls over is a first time
// again.
func (uc *UseCase) settled(ctx context.Context, r resource.Record) error {
	r.Attempts = 0

	_, err := uc.resources.Update(ctx, r)

	return err
}

// lost writes down that a resource's node has gone quiet. One that ended
// already is left as it was.
func (uc *UseCase) lost(ctx context.Context, d kind.Descriptor, r resource.Record, common kind.Status, now time.Time) error {
	if d.Machine.IsTerminal(common.State) {
		return nil
	}

	uc.logger.WarnContext(ctx, "a resource's node has gone quiet", "kind", d.Name, "uuid", r.Metadata.UUID, "node", r.Metadata.Node, "state", common.State)

	if err := observe.Fail(&r, ReasonNodeLost, now); err != nil {
		return err
	}

	r.Metadata.UpdatedAt = now

	_, err := uc.resources.Update(ctx, r)

	return err
}

// inFlight waits on a resource on its way somewhere, and asks once more for
// what it has been on its way to for longer than it takes.
func (uc *UseCase) inFlight(ctx context.Context, d kind.Descriptor, r resource.Record, common kind.Status, now time.Time) error {
	if r.Pending == nil {
		if now.Sub(common.Since) < uc.config.Patience {
			return nil
		}

		uc.logger.WarnContext(ctx, "failing a resource that waited in flight on nothing", "kind", d.Name, "uuid", r.Metadata.UUID, "state", common.State)

		if err := observe.Fail(&r, ReasonStuck, now); err != nil {
			return err
		}

		r.Metadata.UpdatedAt = now

		_, err := uc.resources.Update(ctx, r)

		return err
	}

	if now.Sub(r.Pending.SentAt) < max(uc.config.waited(d, r.Pending.Action), uc.config.backoff(r.Attempts)) {
		return nil
	}

	uc.logger.InfoContext(ctx, "asking again for what a resource did not get to", "kind", d.Name, "uuid", r.Metadata.UUID, "state", common.State, "action", r.Pending.Action, "attempt", r.Attempts)

	asked, err := uc.dispatcher.Again(ctx, r)
	if err != nil {
		return err
	}

	_, err = uc.dispatcher.Send(ctx, *asked.Command, 0)

	return err
}

// remove asks for a resource to be deleted: at once when the state it is in
// allows it, and otherwise by expecting it, so that it is asked once it can
// be. A lifetime ending is something new asked of it; a delete asked for
// again is another try at what was.
func (uc *UseCase) remove(ctx context.Context, binding kind.ControlPlaneBinding, r resource.Record, common kind.Status, fresh bool) error {
	if !binding.Descriptor().Allows(deleteAction, common.State) {
		if common.Expected == kind.Deleted {
			return nil
		}

		_, err := uc.dispatcher.Desire(ctx, r, kind.Deleted, fresh)

		return err
	}

	return uc.ask(ctx, binding, r, common, []kind.Intent{{Action: deleteAction, Reason: "it is to be deleted"}}, fresh)
}

// ask asks a resource for what its kind asked for, in order, until one of
// them is a command for its node: one command is in flight at a time, and
// what is asked after it is asked once it arrives.
func (uc *UseCase) ask(ctx context.Context, binding kind.ControlPlaneBinding, r resource.Record, common kind.Status, intents []kind.Intent, fresh bool) error {
	d := binding.Descriptor()

	for _, intent := range intents {
		if !d.Allows(intent.Action, common.State) {
			uc.logger.WarnContext(ctx, "a kind asked for what its resource's state does not allow", "kind", d.Name, "uuid", r.Metadata.UUID, "action", intent.Action, "state", common.State)

			return nil
		}

		payload, err := dispatch.PayloadOf(intent)
		if err != nil {
			return err
		}

		uc.logger.InfoContext(ctx, "asking a resource for what would make it what it is expected to be", "kind", d.Name, "uuid", r.Metadata.UUID, "action", intent.Action, "reason", intent.Reason, "state", common.State, "expected", common.Expected, "attempt", r.Attempts)

		asked, invalid, err := uc.dispatcher.Ask(ctx, binding, r, intent.Action, payload, fresh)
		if err != nil {
			return err
		}

		if len(invalid) > 0 {
			uc.logger.WarnContext(ctx, "a kind asked for what its resource refused", "kind", d.Name, "uuid", r.Metadata.UUID, "action", intent.Action, "invalid", invalid)

			return nil
		}

		if asked.Gone {
			return nil
		}

		if asked.Command != nil {
			_, err := uc.dispatcher.Send(ctx, *asked.Command, 0)

			return err
		}

		r = asked.Record

		if common, err = r.Common(); err != nil {
			return err
		}
	}

	return nil
}
