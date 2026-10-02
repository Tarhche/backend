package vmhost

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// keeper looks after one running VM: it keeps what the task writes, it
// notices when the task ends, and it does what the task's restart policy says
// then — start it again inside its machine, or let the machine go.
//
// Docker did all of this itself. A microVM has nobody but vmhost to do it, and
// nothing a keeper does survives vmhost going away but the VM's record and its
// output: a keeper is made again for every VM still running when a vmhost
// comes back, and takes up where the last one left off. Ported from PR #101.
type keeper struct {
	engine *Engine
	id     string
	client vm.GuestClient
	out    vm.LogWriter

	// base is what this boot's output is numbered after.
	base uint64

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// wake cuts short a wait to start the task again, once the VM has been
	// stopped meanwhile and is not to be started at all.
	wake chan struct{}

	// finalized says the VM has been let go of, which is done once.
	finalized atomic.Bool

	released sync.Once
}

// attach starts looking after a running VM.
func (e *Engine) attach(v vm.VM, client vm.GuestClient, out vm.LogWriter) *keeper {
	ctx, cancel := context.WithCancel(e.ctx)

	k := &keeper{
		engine: e,
		id:     v.ID,
		client: client,
		out:    out,
		base:   v.LogBase,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		wake:   make(chan struct{}, 1),
	}

	e.lock.Lock()
	e.keepers[k.id] = k
	e.lock.Unlock()

	go k.run()

	return k
}

func (k *keeper) run() {
	defer close(k.done)

	following := make(chan struct{})
	go func() {
		defer close(following)
		k.follow()
	}()
	defer func() { <-following }()
	defer k.cancel()

	for {
		v, err := k.engine.states.Get(k.ctx, k.id)
		if err != nil {
			return
		}

		status, err := k.client.Wait(k.ctx, v.Generation)
		if k.ctx.Err() != nil {
			// vmhost is going away, or the VM is being taken away from
			// this keeper: the machine stays as it is.
			return
		}

		if err != nil {
			k.engine.metrics.agentFailed(k.ctx, "wait")

			if k.engine.alive(k.ctx, k.id) {
				if !k.sleep(k.engine.timing.retry) {
					return
				}

				continue
			}

			// the machine went away under its task.
			k.engine.logger.Warn("a vm's machine went away while its task ran", "vm", k.id, "error", err)
			k.engine.finalize(k, guest.Status{ExitCode: lostExitCode, FinishedAt: time.Now().UTC()}, true)

			return
		}

		// a run newer than the one waited for is running: a vmhost went away
		// after the agent started the task again and before that was written
		// down. It is the run to look after.
		if status.Generation > v.Generation && status.State == guest.StateRunning {
			_, _ = k.engine.states.Update(k.ctx, k.id, func(v *vm.VM) {
				v.State = vm.StateRunning
				v.Generation = status.Generation
				v.StartedAt = status.StartedAt
				v.ExitCode = 0
			})

			continue
		}

		k.drain()

		v, err = k.engine.states.Get(k.ctx, k.id)
		if err != nil {
			return
		}

		if !vm.ParseRestartPolicy(v.Spec.RestartPolicy).Restarts(status.ExitCode, v.RestartCount, v.Stopped) {
			k.engine.finalize(k, status, false)

			return
		}

		if !k.restart(v, status) {
			return
		}
	}
}

// restart starts the task again inside its machine, after the wait its
// restart policy calls for, and reports whether there is more to look after.
func (k *keeper) restart(v vm.VM, ended guest.Status) bool {
	_, _ = k.engine.states.Update(k.ctx, k.id, func(v *vm.VM) {
		v.State = vm.StateRestarting
		v.ExitCode = ended.ExitCode
		v.FinishedAt = ended.FinishedAt
	})

	if !k.sleep(k.engine.timing.backoff(v.RestartCount)) {
		return false
	}

	// stopped while it waited to start again.
	current, err := k.engine.states.Get(k.ctx, k.id)
	if err != nil {
		return false
	}

	if current.Stopped {
		k.engine.finalize(k, ended, false)

		return false
	}

	started, err := k.client.Start(k.ctx, current.Process)
	if err != nil {
		if k.ctx.Err() != nil {
			return false
		}

		k.engine.metrics.agentFailed(k.ctx, "start")
		k.engine.logger.Warn("a vm's task could not be started again", "vm", k.id, "error", err)
		k.engine.finalize(k, ended, false)

		return false
	}

	startedAt := started.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	_, _ = k.engine.states.Update(k.ctx, k.id, func(v *vm.VM) {
		v.State = vm.StateRunning
		v.Generation = started.Generation
		v.RestartCount++
		v.StartedAt = startedAt
		v.FinishedAt = time.Time{}
		v.ExitCode = 0
	})

	k.engine.metrics.restarted(k.ctx, "task")

	return true
}

// follow keeps what the task writes, as it writes it, for as long as the
// keeper runs. The agent keeps what nobody read yet, so a follower that has
// to start again loses nothing.
func (k *keeper) follow() {
	for k.ctx.Err() == nil {
		_ = k.client.Logs(k.ctx, k.after(), true, k.keep)

		select {
		case <-time.After(k.engine.timing.retry):
		case <-k.ctx.Done():
		}
	}
}

// drain keeps whatever the task wrote last, once it has ended.
func (k *keeper) drain() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(k.ctx), k.engine.timing.drain)
	defer cancel()

	if err := k.client.Logs(ctx, k.after(), false, k.keep); err != nil {
		k.engine.metrics.agentFailed(ctx, "logs")
		k.engine.logger.Warn("what a vm's task wrote last could not all be read", "vm", k.id, "error", err)
	}
}

// after is the last line the agent numbered that is kept already.
func (k *keeper) after() uint64 {
	last := k.out.Last()
	if last < k.base {
		return 0
	}

	return last - k.base
}

// keep keeps a line, numbered after everything the VM wrote before this boot.
func (k *keeper) keep(line guest.LogLine) error {
	return k.out.Add(vm.LogLine{
		Seq:     line.Seq + k.base,
		Stream:  line.Stream,
		At:      line.At,
		Content: line.Content,
	})
}

// sleep waits for d, or until it is woken, and reports whether the keeper is
// still running.
func (k *keeper) sleep(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-k.wake:
		return true
	case <-k.ctx.Done():
		return false
	}
}

// wakeUp cuts short a wait the keeper is in.
func (k *keeper) wakeUp() {
	select {
	case k.wake <- struct{}{}:
	default:
	}
}

// abandon stops looking after the VM, and leaves its machine as it is.
func (k *keeper) abandon() {
	k.cancel()
	<-k.done
}

// release lets go of the agent's connections and of the VM's output.
func (k *keeper) release() {
	k.released.Do(func() {
		if err := k.client.Close(); err != nil {
			k.engine.logger.Debug("a vm's agent could not be let go of", "vm", k.id, "error", err)
		}

		if err := k.out.Close(); err != nil {
			k.engine.logger.Warn("a vm's output could not be closed", "vm", k.id, "error", err)
		}
	})
}

// finalize lets go of a VM whose task has ended: its record says what the
// task ended with, the machine is turned off, and what it held is given back.
//
// A machine that went away on its own is booted again if the VM's restart
// policy says so — saying restarting meanwhile rather than dead, since dead is
// what the orchestrator takes for a task that failed — and is dead otherwise.
func (e *Engine) finalize(k *keeper, ended guest.Status, lost bool) {
	if !k.finalized.CompareAndSwap(false, true) {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), e.timing.finalize)
	defer cancel()

	finished := ended.FinishedAt
	if finished.IsZero() {
		finished = time.Now().UTC()
	}

	restarting := e.isRestarting(k.id)

	var (
		held   []vm.Interface
		reboot bool
	)

	v, err := e.states.Update(ctx, k.id, func(v *vm.VM) {
		held = v.Interfaces

		v.ExitCode = ended.ExitCode
		v.FinishedAt = finished
		v.Interfaces = nil

		switch {
		case restarting:
			v.State = vm.StateRestarting
		case lost && vm.ParseRestartPolicy(v.Spec.RestartPolicy).Restarts(lostExitCode, v.RestartCount, v.Stopped):
			v.State = vm.StateRestarting
			v.Reason = reasonLost + ", and is booted again"
			reboot = true
		case lost:
			v.State = vm.StateDead
			v.Reason = reasonLost
		default:
			v.State = vm.StateExited
		}
	})
	if err != nil && !errors.Is(err, vm.ErrNotFound) {
		e.logger.Error("what a vm's task ended with could not be kept", "vm", k.id, "error", err)
	}

	// a machine turns itself off, which puts its disks away as they should
	// be; one that does not in time is ended from outside.
	if !lost {
		asked, cancelAsking := context.WithTimeout(ctx, e.timing.settle)
		if err := k.client.PowerOff(asked); err == nil {
			e.waitGone(asked, k.id)
		}
		cancelAsking()
	}

	if err := e.hypervisor.Terminate(ctx, k.id); err != nil {
		e.logger.Error("a vm's machine could not be let go", "vm", k.id, "error", err)
	}

	if err := e.fabric.Unplug(ctx, k.id); err != nil {
		e.logger.Error("what a vm's machine held could not be given back", "vm", k.id, "error", err)
	}

	e.detach(k)
	k.release()
	e.forgetSample(k.id)

	e.refreshHosts(ctx, k.id, held)

	e.logger.Info("vm stopped", "vm", k.id, "exit_code", ended.ExitCode, "lost", lost)

	if err != nil {
		return
	}

	switch {
	case reboot:
		e.scheduleReboot(k.id, e.timing.backoff(v.RestartCount))
	case v.Spec.AutoRemove && !restarting:
		e.later(func() {
			if err := e.Delete(e.ctx, k.id); err != nil && !errors.Is(err, vm.ErrNotFound) {
				e.logger.Error("a vm that removes itself could not be removed", "vm", k.id, "error", err)
			}
		})
	}
}

// reasonLost is why a VM whose machine went away is where it is.
const reasonLost = "the machine went away under its task"

// alive reports whether a VM's machine still runs, which is how a machine
// that stopped answering is told from one that is gone.
func (e *Engine) alive(ctx context.Context, id string) bool {
	machine, err := e.hypervisor.Machine(ctx, id)
	if errors.Is(err, vm.ErrNotFound) {
		return false
	}

	if err != nil {
		// the hypervisor not answering says nothing about the machine.
		return true
	}

	return machine.Running
}

// waitGone waits a moment for a machine that was asked to turn off to be off.
func (e *Engine) waitGone(ctx context.Context, id string) {
	for e.alive(ctx, id) {
		select {
		case <-time.After(e.timing.gone):
		case <-ctx.Done():
			return
		}
	}
}

// pendingReboot is a VM waiting to be booted again: the timer that boots it,
// and which wait it is, so that a wait cancelled and begun again is not taken
// for the one that ended.
type pendingReboot struct {
	timer *time.Timer
	seq   uint64
}

// scheduleReboot boots a VM whose machine went away again, once it has waited
// as long as its restart policy says.
func (e *Engine) scheduleReboot(id string, delay time.Duration) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.closed {
		return
	}

	if pending, found := e.pending[id]; found {
		pending.timer.Stop()
	}

	e.rebootSeq++
	seq := e.rebootSeq

	// the timer is set while the lock is held, and what it runs takes the
	// lock first, so it always finds itself written down.
	e.pending[id] = &pendingReboot{
		seq:   seq,
		timer: time.AfterFunc(delay, func() { e.later(func() { e.rebootNow(id, seq) }) }),
	}
}

// rebootNow boots a VM whose wait to be booted again is over, unless it has
// been stopped, started or deleted meanwhile.
func (e *Engine) rebootNow(id string, seq uint64) {
	unlock := e.locks.lock(id)
	defer unlock()

	e.lock.Lock()
	pending, found := e.pending[id]
	current := found && pending.seq == seq
	if current {
		delete(e.pending, id)
	}
	e.lock.Unlock()

	if !current {
		return
	}

	v, err := e.states.Get(e.ctx, id)
	if err != nil || v.State != vm.StateRestarting || v.Stopped || e.keeper(id) != nil {
		return
	}

	e.reboot(e.ctx, id)
}

// reboot boots a VM again because its restart policy says so, which counts as
// one of its restarts. A VM that cannot be booted again is dead. The VM's lock
// is held.
func (e *Engine) reboot(ctx context.Context, id string) {
	if _, err := e.states.Update(ctx, id, func(v *vm.VM) { v.RestartCount++ }); err != nil {
		return
	}

	e.metrics.restarted(ctx, "machine")

	if err := e.start(ctx, id, vm.StateDead); err != nil {
		e.logger.Error("a vm whose machine went away could not be booted again", "vm", id, "error", err)
	}
}
