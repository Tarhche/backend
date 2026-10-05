// Package lifecycle moves VMs between the states a person, or the control
// plane's own heartbeat, asks for.
//
// What is asked of a VM is written down first and the node is asked second,
// always: a VM whose node never hears of it is brought back by the heartbeat,
// which reads what was written, while a node that hears of something nobody
// wrote down is one nothing will ever correct.
//
// A VM that is on its way somewhere — being made, started, stopped, restarted
// or restored — is not asked for anything else until it gets there. What is
// wanted of it is written down, and the heartbeat asks for that once the node
// reports it has arrived: commands on different subjects reach a node in no
// particular order, so a stop and the start that followed it could otherwise be
// carried out the other way round.
package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/placement"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// NodeSilentAfter is how long a node may go unheard before it is taken to
	// be gone. Nodes speak every second, so this is many missed heartbeats.
	NodeSilentAfter = 30 * time.Second

	// VMSilentAfter is how long a node may go without listing one of its VMs
	// before the VM is taken to be no longer there. A node lists its VMs less
	// often than it says it is alive, so this is longer.
	VMSilentAfter = time.Minute

	// ReasonNoCapacity is why a VM no node had room for failed.
	ReasonNoCapacity = "no_capacity"

	// ReasonNodeLost is why a VM whose node went quiet failed. It is not given
	// up on: a node that comes back reports its VMs, and those that are meant
	// to be running are started again.
	ReasonNodeLost = "node_lost"
)

// ErrBusy is a VM on its way out, which nothing can be asked of any more.
var ErrBusy = errors.New("the vm is being deleted")

// Lifecycle moves VMs between states.
type Lifecycle struct {
	vms       vm.Repository
	stacks    stack.Repository
	nodes     node.Repository
	placement *placement.Placement
	commander *command.Commander

	now func() time.Time
}

func New(
	vms vm.Repository,
	stacks stack.Repository,
	nodes node.Repository,
	placement *placement.Placement,
	commander *command.Commander,
) *Lifecycle {
	return &Lifecycle{
		vms:       vms,
		stacks:    stacks,
		nodes:     nodes,
		placement: placement,
		commander: commander,
		now:       time.Now,
	}
}

// Now is the time the lifecycle goes by.
func (l *Lifecycle) Now() time.Time {
	return l.now()
}

// Listed reports whether a VM's node has listed it lately, which is how the
// control plane knows the node holds an instance of it to start rather than one
// to make.
func Listed(v *vm.VM, now time.Time) bool {
	return !v.LastHeartbeatAt.IsZero() && now.Sub(v.LastHeartbeatAt) <= VMSilentAfter
}

// NodeAlive reports whether a node has spoken lately.
func (l *Lifecycle) NodeAlive(ctx context.Context, nodeName string) (bool, error) {
	if len(nodeName) == 0 {
		return false, nil
	}

	n, err := l.nodes.GetOne(ctx, nodeName)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return l.now().Sub(n.LastHeartbeatAt) <= NodeSilentAfter, nil
}

// Up asks for a VM to be running.
//
// A VM on no node is placed first, and failed when no node has room for it
// (vm.ErrNoCapacity); its node starts the instance it holds, or makes one when
// it holds none, which is also what brings back one that was running and is no
// longer listed. A VM on its way somewhere, or on a node that has gone quiet,
// is only told what is wanted of it: the heartbeat asks once it can be asked.
func (l *Lifecycle) Up(ctx context.Context, v *vm.VM) error {
	if v.CurrentState == vm.Deleting {
		return ErrBusy
	}

	now := l.now()
	v.ExpectedState = vm.Running
	v.UpdatedAt = now

	if len(v.NodeName) == 0 {
		return l.place(ctx, v)
	}

	if vm.IsInFlightState(v.CurrentState) {
		return l.save(ctx, v)
	}

	alive, err := l.NodeAlive(ctx, v.NodeName)
	if err != nil {
		return err
	}

	// a node that has gone quiet cannot be asked, and one that is running is
	// where it was asked to be.
	if !alive || (v.CurrentState == vm.Running && Listed(v, now)) {
		return l.save(ctx, v)
	}

	v.Reason = ""

	if Listed(v, now) {
		v.CurrentState = vm.Starting
		if err := l.save(ctx, v); err != nil {
			return err
		}

		return l.commander.Start(ctx, v)
	}

	v.CurrentState = vm.Scheduled
	if err := l.save(ctx, v); err != nil {
		return err
	}

	return l.commander.Schedule(ctx, v, v.RestoreFrom)
}

// place chooses a node for a VM that has none, and asks it to make the VM.
func (l *Lifecycle) place(ctx context.Context, v *vm.VM) error {
	chosen, err := l.placement.Pick(ctx, v.Resources)

	switch {
	case errors.Is(err, vm.ErrNoCapacity):
		// failed, and given up on, rather than asked for again and again:
		// whoever wants it asks again once there is room.
		v.CurrentState = vm.Failed
		v.ExpectedState = vm.Failed
		v.Reason = ReasonNoCapacity

		if err := l.save(ctx, v); err != nil {
			return err
		}

		return vm.ErrNoCapacity
	case err != nil:
		return err
	}

	v.NodeName = chosen.Name
	v.CurrentState = vm.Scheduled
	v.Reason = ""

	if err := l.save(ctx, v); err != nil {
		return err
	}

	return l.commander.Schedule(ctx, v, v.RestoreFrom)
}

// Down asks for a VM to be stopped. One with nothing running anywhere is
// stopped already, which is written down rather than asked for.
func (l *Lifecycle) Down(ctx context.Context, v *vm.VM) error {
	if v.CurrentState == vm.Deleting {
		return ErrBusy
	}

	now := l.now()
	v.ExpectedState = vm.Stopped
	v.UpdatedAt = now

	if vm.IsInFlightState(v.CurrentState) && v.CurrentState != vm.Created {
		return l.save(ctx, v)
	}

	alive, err := l.NodeAlive(ctx, v.NodeName)
	if err != nil {
		return err
	}

	switch {
	case len(v.NodeName) == 0, v.CurrentState == vm.Stopped, v.CurrentState == vm.Failed:
		if v.CurrentState != vm.Failed {
			v.CurrentState = vm.Stopped
		}

		return l.save(ctx, v)

	case !alive:
		return l.save(ctx, v)

	case !Listed(v, now):
		// its node holds nothing of it, so nothing is running.
		v.CurrentState = vm.Stopped

		return l.save(ctx, v)
	}

	v.CurrentState = vm.Stopping
	if err := l.save(ctx, v); err != nil {
		return err
	}

	return l.commander.Stop(ctx, v)
}

// Restart asks for a running VM to be stopped and booted again in place. One
// that is not running is brought up instead, which is what restarting it
// amounts to.
func (l *Lifecycle) Restart(ctx context.Context, v *vm.VM) error {
	if v.CurrentState == vm.Deleting {
		return ErrBusy
	}

	now := l.now()

	if v.CurrentState != vm.Running || len(v.NodeName) == 0 || !Listed(v, now) {
		return l.Up(ctx, v)
	}

	alive, err := l.NodeAlive(ctx, v.NodeName)
	if err != nil {
		return err
	}

	v.ExpectedState = vm.Running
	v.UpdatedAt = now

	if !alive {
		return l.save(ctx, v)
	}

	v.CurrentState = vm.Restarting
	if err := l.save(ctx, v); err != nil {
		return err
	}

	return l.commander.Restart(ctx, v)
}

// Reconfigure writes down the ports, network and resources a VM now has, and
// asks its node to apply them to the instance it holds, which restarts a
// running VM when the engine has to. A VM whose node holds nothing of it is
// made with them when it is next brought up.
func (l *Lifecycle) Reconfigure(ctx context.Context, v *vm.VM) error {
	now := l.now()
	v.UpdatedAt = now

	alive, err := l.NodeAlive(ctx, v.NodeName)
	if err != nil {
		return err
	}

	if !alive || !Listed(v, now) {
		return l.save(ctx, v)
	}

	if v.CurrentState == vm.Running {
		v.CurrentState = vm.Restarting
	}

	if err := l.save(ctx, v); err != nil {
		return err
	}

	return l.commander.Reconfigure(ctx, v)
}

// Remove asks for a VM to be deleted. One that is on no node, or on a node
// that has gone quiet, has nothing to wait for and is forgotten at once: a node
// that comes back holding it is asked to remove it, as it is for anything it
// holds that the control plane has no record of.
func (l *Lifecycle) Remove(ctx context.Context, v *vm.VM) error {
	alive, err := l.NodeAlive(ctx, v.NodeName)
	if err != nil {
		return err
	}

	if !alive {
		return l.Forget(ctx, v.UUID)
	}

	v.CurrentState = vm.Deleting
	v.ExpectedState = vm.Deleting
	v.UpdatedAt = l.now()

	if err := l.save(ctx, v); err != nil {
		return err
	}

	return l.commander.Delete(ctx, v.UUID, v.NodeName)
}

// Forget takes away the record of a VM its node no longer holds, and the
// stacks that were deployed into it, which went with its disk. Its snapshots
// stay: they outlive it.
//
// The stacks go first, so that one cut short leaves a VM with fewer stacks
// rather than stacks in a VM that is not there.
func (l *Lifecycle) Forget(ctx context.Context, uuid string) error {
	stacks, err := l.stacks.GetAllByVM(ctx, uuid)
	if err != nil {
		return err
	}

	for i := range stacks {
		if err := l.stacks.Delete(ctx, stacks[i].UUID); err != nil {
			return err
		}
	}

	return l.vms.Delete(ctx, uuid)
}

func (l *Lifecycle) save(ctx context.Context, v *vm.VM) error {
	_, err := l.vms.Save(ctx, v)

	return err
}

// Refused is what a person is told when a VM cannot be asked for what they
// asked: a VM on its way out, or one no node has room for. Anything else is an
// error rather than a refusal, and is handed back as one.
func Refused(err error) (domain.ValidationErrors, error) {
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, ErrBusy):
		return domain.ValidationErrors{"vm": "invalid_state_transition"}, nil
	case errors.Is(err, vm.ErrNoCapacity):
		return domain.ValidationErrors{"vm": ReasonNoCapacity}, nil
	default:
		return nil, err
	}
}
