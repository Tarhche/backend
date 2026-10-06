// Package reconcile brings VMs back to what was asked of them.
//
// What a VM should be doing is written down; what it is doing is what its node
// last reported. A VM stopped from inside, one that fell over, a node that
// restarted and stopped everything it held, a command that never arrived — all
// leave the two disagreeing, and this is the control plane's own heartbeat
// looking at that disagreement, pass after pass, and asking the node holding
// each VM for the one thing that would close it. It is the same heartbeat that
// brings tasks back, extended.
//
// A VM on its way somewhere is given time to get there before it is asked
// again; one on a node that has gone quiet is not asked anything, and is
// failed as lost until its node comes back and reports it. One whose lifetime
// is over is deleted, and one whose deletion its node never confirmed is asked
// for again.
//
// What lives in a VM, its stacks, is brought back by its own kind's
// reconcile, once its VM runs.
package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// batch is how many records are read at a time. A pass works through
	// every one of them, a batch at a time, so that how many there are decides
	// how long a pass takes rather than whether it covers them all.
	batch uint = 20

	// nodesLimit is the most nodes looked at. A workload has a handful.
	nodesLimit uint = 100

	// failedBackoff is how long a VM that fell over is left before it is
	// brought back, so that one that falls over at once is not brought back on
	// every pass.
	failedBackoff = time.Minute

	// deletePatience is how long a VM's node is given to confirm it removed
	// it before it is asked again.
	deletePatience = 2 * time.Minute
)

// patience is how long a VM may be on its way somewhere before it is asked
// again. Making one may pull its image first, and restoring one downloads its
// snapshot, so those are given the longest.
func patience(state vm.State) time.Duration {
	switch state {
	case vm.Created:
		return time.Minute
	case vm.Scheduled:
		return 10 * time.Minute
	case vm.Restoring:
		return 30 * time.Minute
	default:
		return 5 * time.Minute
	}
}

// UseCase is one pass over the VMs.
type UseCase struct {
	vms       vm.Repository
	nodes     node.Repository
	lifecycle *lifecycle.Lifecycle
	commander *command.Commander
	logger    *slog.Logger
}

func NewUseCase(
	vms vm.Repository,
	nodes node.Repository,
	lifecycle *lifecycle.Lifecycle,
	commander *command.Commander,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		vms:       vms,
		nodes:     nodes,
		lifecycle: lifecycle,
		commander: commander,
		logger:    logger,
	}
}

// Execute looks at every VM, and asks for what is missing. One that cannot be dealt with is not a reason to leave the rest as
// they are: it is reported, and the next pass tries it again.
//
// What is read moves while it is read — a VM deleted during a pass shifts the
// rest along — so one may be looked at twice, which asks for what it needs
// twice and is the same answer, or missed, which the next pass picks up.
func (uc *UseCase) Execute(ctx context.Context) error {
	// what they are judged against is when the pass began, so that a VM is
	// not called late for the time a long pass took to reach it.
	now := uc.lifecycle.Now()

	alive, err := uc.aliveNodes(ctx, now)
	if err != nil {
		return err
	}

	count, err := uc.vms.Count(ctx)
	if err != nil {
		return err
	}

	for offset := uint(0); offset < count; offset += batch {
		vms, err := uc.vms.GetAll(ctx, offset, batch)
		if err != nil {
			return err
		}

		if len(vms) == 0 {
			break
		}

		for i := range vms {
			if err := uc.look(ctx, &vms[i], now, alive); err != nil {
				uc.logger.ErrorContext(ctx, "could not bring a vm back to what was asked of it",
					"error", err, "uuid", vms[i].UUID, "expected", vms[i].ExpectedState.String(), "current", vms[i].CurrentState.String())
			}
		}
	}

	return nil
}

// aliveNodes is which nodes have spoken lately.
func (uc *UseCase) aliveNodes(ctx context.Context, now time.Time) (map[string]bool, error) {
	nodes, err := uc.nodes.GetAll(ctx, 0, nodesLimit)
	if err != nil {
		return nil, err
	}

	alive := make(map[string]bool, len(nodes))
	for i := range nodes {
		alive[nodes[i].Name] = now.Sub(nodes[i].LastHeartbeatAt) <= lifecycle.NodeSilentAfter
	}

	return alive, nil
}

// look asks for what one VM is missing, if it is missing anything.
func (uc *UseCase) look(ctx context.Context, v *vm.VM, now time.Time, alive map[string]bool) error {
	placed := len(v.NodeName) > 0
	nodeAlive := placed && alive[v.NodeName]

	switch {
	case v.CurrentState == vm.Deleting:
		return uc.deleting(ctx, v, now, nodeAlive)

	case v.Expired(now):
		uc.logger.InfoContext(ctx, "deleting a vm that has outlived its lifetime", "uuid", v.UUID, "expires_at", v.ExpiresAt)

		return uc.lifecycle.Remove(ctx, v)

	case placed && !nodeAlive:
		return uc.lost(ctx, v)

	case vm.IsInFlightState(v.CurrentState):
		if now.Sub(v.UpdatedAt) <= patience(v.CurrentState) {
			return nil
		}

		return uc.again(ctx, v, now)
	}

	switch v.ExpectedState {
	case vm.Running:
		switch {
		case v.CurrentState == vm.Running && lifecycle.Listed(v, now):
			return nil
		case v.CurrentState == vm.Failed && now.Sub(v.UpdatedAt) <= failedBackoff:
			return nil
		}

		uc.logger.InfoContext(ctx, "a vm is not running as it was asked to be", "uuid", v.UUID, "current", v.CurrentState.String())

		// its node starts the instance it holds, or makes one when it holds
		// none: a VM stopped from inside, one that fell over and one its node
		// no longer lists all come back the same way.
		return uc.lifecycle.Up(ctx, v)

	case vm.Stopped:
		if v.CurrentState == vm.Running {
			return uc.lifecycle.Down(ctx, v)
		}
	}

	return nil
}

// deleting asks again for a VM whose node has not confirmed its deletion, and
// forgets one whose node is gone: a node that comes back holding it is asked to
// remove it then.
func (uc *UseCase) deleting(ctx context.Context, v *vm.VM, now time.Time, nodeAlive bool) error {
	if !nodeAlive {
		return uc.lifecycle.Forget(ctx, v.UUID)
	}

	if now.Sub(v.UpdatedAt) <= deletePatience {
		return nil
	}

	v.UpdatedAt = now
	if _, err := uc.vms.Save(ctx, v); err != nil {
		return err
	}

	return uc.commander.Delete(ctx, v.UUID, v.NodeName)
}

// lost writes down that a VM's node has gone quiet. Nothing is asked of it,
// since nothing can be, and what is wanted of it stays as it was: a node that
// comes back reports the VM, and one wanted running is brought back then. A
// VM that was resting already is left as it was.
func (uc *UseCase) lost(ctx context.Context, v *vm.VM) error {
	if v.CurrentState == vm.Stopped || v.CurrentState == vm.Failed {
		return nil
	}

	uc.logger.WarnContext(ctx, "a vm's node has gone quiet", "uuid", v.UUID, "node", v.NodeName)

	v.CurrentState = vm.Failed
	v.Reason = lifecycle.ReasonNodeLost

	_, err := uc.vms.Save(ctx, v)

	return err
}

// again asks once more for what a VM has been on its way to for longer than it
// takes.
func (uc *UseCase) again(ctx context.Context, v *vm.VM, now time.Time) error {
	uc.logger.InfoContext(ctx, "asking again for a vm that did not get where it was going", "uuid", v.UUID, "current", v.CurrentState.String())

	if v.CurrentState == vm.Created {
		return uc.lifecycle.Up(ctx, v)
	}

	if v.CurrentState == vm.Restoring && len(v.RestoreFrom) == 0 {
		v.CurrentState = vm.Failed
		v.Reason = "the restore was lost"

		_, err := uc.vms.Save(ctx, v)

		return err
	}

	// a VM its node no longer holds has nothing to stop, and is made again
	// whatever else it was on its way to.
	if !lifecycle.Listed(v, now) {
		switch v.CurrentState {
		case vm.Stopping:
			v.CurrentState = vm.Stopped

			_, err := uc.vms.Save(ctx, v)

			return err
		case vm.Starting, vm.Restarting:
			v.CurrentState = vm.Scheduled
		}
	}

	v.UpdatedAt = now
	if _, err := uc.vms.Save(ctx, v); err != nil {
		return err
	}

	switch v.CurrentState {
	case vm.Scheduled:
		return uc.commander.Schedule(ctx, v, v.RestoreFrom)
	case vm.Starting:
		return uc.commander.Start(ctx, v)
	case vm.Stopping:
		return uc.commander.Stop(ctx, v)
	case vm.Restarting:
		return uc.commander.Restart(ctx, v)
	case vm.Restoring:
		return uc.commander.Restore(ctx, v, v.RestoreFrom)
	}

	return nil
}
