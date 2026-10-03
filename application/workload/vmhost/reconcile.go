package vmhost

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Reconcile holds what vmhost's records say against what the host holds, and
// puts right what differs. vmhost does it when it starts, before it takes any
// request, and every few seconds after:
//
//   - a VM still running that nobody looks after — one an earlier vmhost
//     booted — is taken back: its agent is reached again and its output
//     followed from the last line kept, which the agent kept meanwhile;
//   - a VM whose machine went away while nobody was looking ended with it, and
//     is booted again if its restart policy says so, or is dead;
//   - a machine nothing accounts for any more is ended;
//   - taps and addresses nothing holds are given back, and the firewall is put
//     back as the networks say it should be;
//   - the network public VMs route out through is made, if it is not there;
//   - images no VM boots are let go of, least recently used first, once they
//     take more disk than they may.
//
// A VM that something is being done to right now is passed over: what is
// being done to it is its account of itself. Reconcile reports whether vmhost
// can run VMs, which is what Health says until the next one.
func (e *Engine) Reconcile(ctx context.Context) error {
	if !e.reconciling.TryLock() {
		return nil
	}
	defer e.reconciling.Unlock()

	machines, err := e.hypervisor.Machines(ctx)
	if err != nil {
		err = fmt.Errorf("the machines on this host cannot be told: %w", err)
		e.setHealth(false, err.Error())

		return err
	}

	held := make(map[string]vm.Machine, len(machines))
	for _, machine := range machines {
		held[machine.ID] = machine
	}

	all, err := e.states.All(ctx)
	if err != nil {
		e.setHealth(false, err.Error())

		return err
	}

	var (
		failures []error
		busy     = make(map[string]bool)
		known    = make(map[string]bool, len(all))
	)

	for _, v := range all {
		known[v.ID] = true

		unlock, free := e.locks.tryLock(v.ID)
		if !free {
			busy[v.ID] = true

			continue
		}

		machine, present := held[v.ID]
		if err := e.reconcileVM(ctx, v.ID, machine, present); err != nil {
			failures = append(failures, fmt.Errorf("vm %s: %w", v.ID, err))
		}

		unlock()
	}

	// a machine of a VM nothing remembers: what is left of a vmhost that
	// went away while it deleted it, or of a record that was lost.
	for _, machine := range machines {
		if known[machine.ID] {
			continue
		}

		e.metrics.orphaned(ctx, machine)
		e.logger.WarnContext(ctx, "a machine no vm accounts for is ended", "machine", machine.ID, "unit", machine.Unit)

		if err := e.hypervisor.Terminate(ctx, machine.ID); err != nil {
			failures = append(failures, fmt.Errorf("machine %s: %w", machine.ID, err))
		}
	}

	if err := e.retain(ctx, busy); err != nil {
		failures = append(failures, err)
	}

	unhealthy := e.repair(ctx)

	if err := e.collectImages(ctx); err != nil {
		failures = append(failures, err)
	}

	if unhealthy != nil {
		e.setHealth(false, unhealthy.Error())

		return errors.Join(append(failures, unhealthy)...)
	}

	e.setHealth(true, "")

	return errors.Join(failures...)
}

// reconcileVM puts right what differs for one VM. Its lock is held.
func (e *Engine) reconcileVM(ctx context.Context, id string, machine vm.Machine, present bool) error {
	v, err := e.states.Get(ctx, id)
	if errors.Is(err, vm.ErrNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	if e.keeper(id) != nil {
		return nil
	}

	switch {
	case v.State == vm.StateRemoving:
		// a delete that did not get to the end.
		return e.delete(ctx, id)
	case v.State.Up():
		if e.isPending(id) {
			return nil
		}

		if present && machine.Running {
			return e.adopt(ctx, v, machine)
		}

		return e.lost(ctx, v, present, reasonLost)
	case present:
		// a machine left behind by a VM that does not run: a boot or an
		// end that a vmhost did not see through. Its VM is known, so it is
		// not counted among the machines nothing accounts for.
		e.logger.WarnContext(ctx, "a machine left behind by a vm that does not run is ended", "vm", id, "state", v.State)

		return errors.Join(e.hypervisor.Terminate(ctx, id), e.unplug(ctx, id))
	default:
		return nil
	}
}

// adopt takes back a VM an earlier vmhost booted, whose machine still runs:
// its agent is reached again, and its output followed from the last line kept.
//
// An agent that answers in a protocol this vmhost does not know is refused,
// rather than misread: its VM is dead, with the reason. One that does not
// answer at all is looked after all the same, since its machine runs: its
// keeper asks it again until it answers or its machine goes away. A machine
// whose task was never started — a boot a vmhost did not see through — is
// booted again. The VM's lock is held.
func (e *Engine) adopt(ctx context.Context, v vm.VM, machine vm.Machine) error {
	client := e.guests.Connect(machine.VsockPath)

	ready, cancel := context.WithTimeout(ctx, e.timing.adopt)
	err := client.Ready(ready)
	cancel()

	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), client.Close())
	}

	answering := err == nil

	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		e.metrics.agentFailed(ctx, "ready")
		e.logger.WarnContext(ctx, "the agent of a vm being taken back is refused", "vm", v.ID, "error", err)

		// it cannot be driven, and is let go of as one whose machine went
		// away: its restart policy boots it again, with the agent this
		// vmhost brought along, or it is dead.
		refused := fmt.Errorf("its agent was refused when vmhost took it back: %w", err)

		return errors.Join(client.Close(), e.lost(ctx, v, true, refused.Error()))
	}

	if answering {
		asking, cancel := context.WithTimeout(ctx, e.timing.adopt)
		status, err := client.Status(asking)
		cancel()

		switch {
		case err != nil:
			e.metrics.agentFailed(ctx, "status")
		case status.State == guest.StateCreated:
			e.logger.WarnContext(ctx, "a vm whose boot was not seen through is booted again", "vm", v.ID)

			if _, err := e.states.Update(ctx, v.ID, func(v *vm.VM) { v.State = vm.StateRestarting }); err != nil {
				return errors.Join(err, client.Close())
			}

			if err := errors.Join(client.Close(), e.hypervisor.Terminate(ctx, v.ID), e.unplug(ctx, v.ID)); err != nil {
				return err
			}

			return e.start(ctx, v.ID, vm.StateDead)
		case status.Generation > v.Generation:
			updated, err := e.states.Update(ctx, v.ID, func(v *vm.VM) {
				v.Generation = status.Generation
				v.StartedAt = status.StartedAt
			})
			if err != nil {
				return errors.Join(err, client.Close())
			}

			v = updated
		}
	}

	out, err := e.logs.Writer(v.ID)
	if err != nil {
		return errors.Join(err, client.Close())
	}

	e.attach(v, client, out)
	e.metrics.adopted(ctx)

	e.logger.InfoContext(ctx, "vm taken back", "vm", v.ID, "name", v.Spec.Name, "answering", answering)

	return nil
}

// lost lets go of what is left of a VM whose machine went away while nobody
// was looking, or cannot be driven, and boots it again if its restart policy
// says so; why says why it went. The VM's lock is held.
func (e *Engine) lost(ctx context.Context, v vm.VM, present bool, why string) error {
	e.logger.WarnContext(ctx, "a vm's machine is let go of", "vm", v.ID, "state", v.State, "reason", why)

	var cleared error
	if present {
		cleared = e.hypervisor.Terminate(ctx, v.ID)
	}

	if err := errors.Join(cleared, e.unplug(ctx, v.ID)); err != nil {
		return err
	}

	now := time.Now().UTC()

	if vm.ParseRestartPolicy(v.Spec.RestartPolicy).Restarts(lostExitCode, v.RestartCount, v.Stopped) {
		_, err := e.states.Update(ctx, v.ID, func(v *vm.VM) {
			v.State = vm.StateRestarting
			v.ExitCode = lostExitCode
			v.FinishedAt = now
			v.Reason = reason(fmt.Errorf("%s, and is booted again", why))
		})
		if err != nil {
			return err
		}

		e.reboot(ctx, v.ID)

		return nil
	}

	updated, err := e.states.Update(ctx, v.ID, func(v *vm.VM) {
		v.State = vm.StateDead
		v.ExitCode = lostExitCode
		v.FinishedAt = now
		v.Reason = reason(errors.New(why))
	})
	if err != nil {
		return err
	}

	if updated.Spec.AutoRemove {
		return e.delete(ctx, v.ID)
	}

	return nil
}

// retain gives back the taps and addresses of every machine that holds none
// by its record, and is not being booted or let go of right now.
func (e *Engine) retain(ctx context.Context, busy map[string]bool) error {
	e.plugging.Lock()
	defer e.plugging.Unlock()

	all, err := e.states.All(ctx)
	if err != nil {
		return err
	}

	keep := make([]string, 0, len(all))
	for _, v := range all {
		if len(v.Interfaces) > 0 || busy[v.ID] {
			keep = append(keep, v.ID)
		}
	}

	slices.Sort(keep)

	if err := e.fabric.Retain(ctx, keep); err != nil {
		return fmt.Errorf("what machines that went away held cannot be given back: %w", err)
	}

	return nil
}

// repair makes the network public VMs route out through, until it is there,
// and puts the firewall back as the networks say it should be. What it could
// not do is why vmhost cannot run VMs right now.
func (e *Engine) repair(ctx context.Context) error {
	e.lock.Lock()
	made := e.publicMade
	e.lock.Unlock()

	if !made {
		if _, err := e.fabric.EnsureNetwork(ctx, vm.PublicNetwork, true); err != nil {
			return fmt.Errorf("the network vms route out through cannot be made: %w", err)
		}

		e.lock.Lock()
		e.publicMade = true
		e.lock.Unlock()
	}

	if err := e.fabric.Repair(ctx); err != nil {
		return fmt.Errorf("the firewall cannot be put back as it should be: %w", err)
	}

	return nil
}

// collectImages lets go of the images no VM boots, least recently used
// first, once all of them take more disk than they may. One used moments ago
// is kept: a VM may be about to be made from it. A store that does this itself
// (vm.ImagePruner) is told which images VMs boot and left to it.
func (e *Engine) collectImages(ctx context.Context) error {
	e.admission.Lock()
	defer e.admission.Unlock()

	all, err := e.states.All(ctx)
	if err != nil {
		return err
	}

	booted := make(map[string]bool, len(all))
	keep := make([]string, 0, len(all))

	for _, v := range all {
		if !booted[v.ImageDigest] {
			booted[v.ImageDigest] = true
			keep = append(keep, v.ImageDigest)
		}
	}

	slices.Sort(keep)

	var pruned error

	if pruner, prunes := e.images.(vm.ImagePruner); prunes {
		removed, err := pruner.Prune(ctx, keep)
		if err != nil {
			pruned = fmt.Errorf("the images no vm boots cannot all be let go of: %w", err)
		}

		for _, image := range removed {
			e.logger.InfoContext(ctx, "an image no vm boots is let go of", "digest", image.Digest, "reference", image.Reference, "size", image.Size)
		}
	}

	images, err := e.images.List(ctx)
	if err != nil {
		return errors.Join(pruned, fmt.Errorf("the images kept cannot be listed: %w", err))
	}

	var total int64
	for _, image := range images {
		total += image.Size
	}

	defer func() { e.metrics.imageCache(total) }()

	if _, prunes := e.images.(vm.ImagePruner); prunes {
		return pruned
	}

	if e.config.ImageCacheMax == 0 || uint64(max(total, 0)) <= e.config.ImageCacheMax {
		return nil
	}

	recent := time.Now().Add(-e.timing.imageGrace)

	candidates := slices.DeleteFunc(slices.Clone(images), func(image vm.Image) bool {
		return booted[image.Digest] || image.LastUsedAt.After(recent)
	})

	slices.SortFunc(candidates, func(a vm.Image, b vm.Image) int {
		return a.LastUsedAt.Compare(b.LastUsedAt)
	})

	var failures []error

	for _, image := range candidates {
		if uint64(max(total, 0)) <= e.config.ImageCacheMax {
			break
		}

		if err := e.images.Remove(ctx, image.Digest); err != nil {
			failures = append(failures, fmt.Errorf("image %s: %w", image.Digest, err))

			continue
		}

		total -= image.Size

		e.logger.InfoContext(ctx, "an image no vm boots is let go of", "digest", image.Digest, "reference", image.Reference, "size", image.Size)
	}

	return errors.Join(failures...)
}
