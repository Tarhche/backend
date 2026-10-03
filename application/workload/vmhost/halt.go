package vmhost

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Stop ends a VM's task the way docker stops a container: it is sent TERM,
// given timeout to end on its own, and then ended along with everything it
// started; the machine turns itself off once it has. A task stopped on purpose
// is not started again by its restart policy. Stopping a VM that does not run
// is stopping nothing.
func (e *Engine) Stop(ctx context.Context, id string, timeout time.Duration) (err error) {
	unlock := e.locks.lock(id)
	defer unlock()

	began := time.Now()
	defer func() { e.metrics.stopped(ctx, time.Since(began), err) }()

	return e.halt(ctx, id, false, func(k *keeper) error {
		_, err := k.client.Stop(ctx, timeout)

		return err
	})
}

// Kill ends a VM's task at once, without the grace Stop gives it: it is sent
// KILL, and returns 137. A machine whose agent does not answer is ended from
// outside. Killing a VM that does not run is refused, as it is for a
// container.
func (e *Engine) Kill(ctx context.Context, id string) error {
	unlock := e.locks.lock(id)
	defer unlock()

	return e.halt(ctx, id, true, func(k *keeper) error {
		return k.client.Signal(ctx, int(syscall.SIGKILL))
	})
}

// Restart stops a VM's task and starts it again, in a machine booted anew
// from the same disks: what the task wrote to its root survives, as it does a
// container's restart.
//
// The VM says it is restarting from the moment it is stopped until it runs
// again, as a container does: a task on its way back up has not ended, and
// nothing watching it is to think it has.
func (e *Engine) Restart(ctx context.Context, id string) error {
	unlock := e.locks.lock(id)
	defer unlock()

	if _, err := e.states.Get(ctx, id); err != nil {
		return err
	}

	e.markRestarting(id, true)
	defer e.markRestarting(id, false)

	err := e.halt(ctx, id, false, func(k *keeper) error {
		_, err := k.client.Stop(ctx, vm.DefaultStopTimeout)

		return err
	})
	if err != nil {
		e.settle(ctx, id, vm.StateExited, err)

		return err
	}

	return e.start(ctx, id, vm.StateExited)
}

// halt ends a VM's task on purpose with end, and waits for its machine to be
// let go. A task that ended on purpose is not started again by its restart
// policy. A machine whose agent does not answer is ended from outside. The
// VM's lock is held.
func (e *Engine) halt(ctx context.Context, id string, mustRun bool, end func(*keeper) error) error {
	v, err := e.states.Get(ctx, id)
	if err != nil {
		return err
	}

	k := e.keeper(id)
	if k == nil {
		return e.haltIdle(ctx, v, mustRun)
	}

	if _, err := e.states.Update(ctx, id, func(v *vm.VM) { v.Stopped = true }); err != nil {
		return err
	}

	// a keeper waiting to start the task again is to stop waiting: there
	// is nothing to start.
	k.wakeUp()

	if err := end(k); err != nil && !errors.Is(err, guest.ErrNotRunning) {
		e.metrics.agentFailed(ctx, "stop")
		e.logger.WarnContext(ctx, "a vm's agent did not answer, and its machine is ended from outside", "vm", id, "error", err)

		k.abandon()
		e.finalize(k, guest.Status{ExitCode: lostExitCode, FinishedAt: time.Now().UTC()}, false)

		return nil
	}

	select {
	case <-k.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// haltIdle halts a VM nobody looks after. One waiting to be booted again by
// its restart policy is stopped where it is. One that says it runs is one
// vmhost has not taken back yet: it cannot be reached until it is, and the
// caller is to ask again. The VM's lock is held.
func (e *Engine) haltIdle(ctx context.Context, v vm.VM, mustRun bool) error {
	switch {
	case e.cancelReboot(v.ID) || v.State == vm.StateRestarting:
		_, err := e.states.Update(ctx, v.ID, func(v *vm.VM) {
			v.State = vm.StateExited
			v.Stopped = true
		})

		return err
	case v.State == vm.StateRunning:
		return fmt.Errorf("%w: vm %s has not been taken back by this vmhost yet", vm.ErrUnavailable, v.ID)
	case mustRun:
		return fmt.Errorf("%w: vm %s", vm.ErrNotRunning, v.ID)
	default:
		return nil
	}
}

// Delete takes a VM away, and everything kept for it: its machine, its taps
// and addresses, its record, its scratch disk and its output. A VM that runs
// is ended first, without the grace a stop gives it.
func (e *Engine) Delete(ctx context.Context, id string) error {
	unlock := e.locks.lock(id)
	defer unlock()

	return e.delete(ctx, id)
}

// delete takes a VM away. The VM's lock is held.
func (e *Engine) delete(ctx context.Context, id string) error {
	v, err := e.states.Get(ctx, id)
	if err != nil {
		return err
	}

	if _, err := e.states.Update(ctx, id, func(v *vm.VM) { v.State = vm.StateRemoving }); err != nil {
		return err
	}

	if k := e.keeper(id); k != nil {
		k.abandon()
		e.detach(k)
		k.release()
	}

	e.cancelReboot(id)
	e.forgetSample(id)

	err = errors.Join(
		e.hypervisor.Terminate(ctx, id),
		e.fabric.Unplug(ctx, id),
	)
	if err != nil {
		return err
	}

	if err := e.states.Remove(ctx, id); err != nil {
		return err
	}

	e.refreshHosts(ctx, id, v.Interfaces)

	e.logger.InfoContext(ctx, "vm deleted", "vm", id, "name", v.Spec.Name)

	return nil
}
