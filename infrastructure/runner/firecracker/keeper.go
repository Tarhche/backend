package firecracker

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

const (
	// retryInterval is how long a keeper waits before asking a machine that
	// did not answer again.
	retryInterval = time.Second

	// drainTimeout bounds reading what a task wrote last, once it ended.
	drainTimeout = 10 * time.Second

	// the longest wait between two attempts at taking the machines back.
	longestReconcileWait = 30 * time.Second
)

// keeper looks after one running machine: it keeps what the task writes, it
// notices when the task ends, and it does what the task's restart policy says
// then — start it again inside its machine, or let the machine go.
//
// Docker did all of this itself. A microVM has nobody but the orchestrator to
// do it, and nothing it does survives the orchestrator going away but the
// record and the output: a keeper is made again for every machine still
// running when an orchestrator comes back, and takes up where the last left
// off.
type keeper struct {
	runtime *Runtime
	id      string
	client  *guest.Client
	out     *output

	// base is what this boot's output is numbered after.
	base uint64

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// attach starts looking after a running machine.
func (r *Runtime) attach(rec record, client *guest.Client, out *output) {
	ctx, cancel := context.WithCancel(r.ctx)

	k := &keeper{
		runtime: r,
		id:      rec.Execution.ID,
		client:  client,
		out:     out,
		base:    rec.LogBase,
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	r.lock.Lock()
	r.keepers[k.id] = k
	r.lock.Unlock()

	go k.run()
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
		rec, found := k.runtime.store.get(k.id)
		if !found {
			return
		}

		status, err := k.client.Wait(k.ctx, rec.Generation)
		if k.ctx.Err() != nil {
			// the orchestrator is going away, or somebody else is taking
			// over: the machine stays as it is.
			return
		}

		if err != nil {
			if k.runtime.alive(k.ctx, k.id) {
				k.sleep(retryInterval)

				continue
			}

			// the machine went away under its task.
			k.runtime.logger.Warn("a machine went away while its task ran", "machine", k.id, "error", err)
			k.runtime.finalize(k, guest.Status{ExitCode: lostExitCode, FinishedAt: time.Now().UTC()}, true)

			return
		}

		k.drain()

		rec, _ = k.runtime.store.get(k.id)
		if !parsePolicy(rec.Execution.RestartPolicy).restarts(status.ExitCode, rec.RestartCount, rec.Stopped) {
			k.runtime.finalize(k, status, false)

			return
		}

		if !k.restart(rec, status) {
			return
		}
	}
}

// restart starts the task again inside its machine, after the wait its
// restart policy calls for, and reports whether there is more to look after.
func (k *keeper) restart(rec record, ended guest.Status) bool {
	_, _ = k.runtime.store.update(k.id, func(rec *record) {
		rec.Status = task.StatusRestarting
		rec.ExitCode = ended.ExitCode
	})

	if !k.sleep(backoff(rec.RestartCount)) {
		return false
	}

	// stopped while it waited to start again.
	if current, _ := k.runtime.store.get(k.id); current.Stopped {
		k.runtime.finalize(k, ended, false)

		return false
	}

	started, err := k.client.Start(k.ctx, rec.Process)
	if err != nil {
		if k.ctx.Err() != nil {
			return false
		}

		k.runtime.logger.Warn("a task could not be started again", "machine", k.id, "error", err)
		k.runtime.finalize(k, ended, false)

		return false
	}

	_, _ = k.runtime.store.update(k.id, func(rec *record) {
		rec.Status = task.StatusRunning
		rec.Generation = started.Generation
		rec.RestartCount++
		rec.StartedAt = started.StartedAt
		rec.ExitCode = 0
	})

	return true
}

// follow keeps what the task writes, as it writes it, for as long as the
// keeper runs.
func (k *keeper) follow() {
	for k.ctx.Err() == nil {
		_ = k.client.Logs(k.ctx, k.after(), true, k.keep)

		k.sleep(retryInterval)
	}
}

// drain keeps whatever the task wrote last, once it has ended.
func (k *keeper) drain() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(k.ctx), drainTimeout)
	defer cancel()

	if err := k.client.Logs(ctx, k.after(), false, k.keep); err != nil {
		k.runtime.logger.Warn("what a task wrote last could not all be read", "machine", k.id, "error", err)
	}
}

// after is the last line the agent numbered that is kept already.
func (k *keeper) after() uint64 {
	last := k.out.lastSeq()
	if last < k.base {
		return 0
	}

	return last - k.base
}

// keep keeps a line, numbered after everything the machine wrote before this
// boot.
func (k *keeper) keep(line guest.LogLine) error {
	line.Seq += k.base

	return k.out.add(line)
}

// sleep waits for d, and reports whether the keeper is still running.
func (k *keeper) sleep(d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-k.ctx.Done():
		return false
	}
}

// abandon stops looking after the machine, and leaves it as it is.
func (k *keeper) abandon() {
	k.cancel()
	<-k.done
}

func (k *keeper) close() {
	k.client.Close()
	_ = k.out.close()
}

// finalize lets go of a machine whose task has ended: its record says what
// the task ended with, the machine is turned off, and what it held is given
// back. A machine that went away on its own is dead rather than exited.
func (r *Runtime) finalize(k *keeper, ended guest.Status, lost bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.ctx), finalizeTimeout)
	defer cancel()

	finished := ended.FinishedAt
	if finished.IsZero() {
		finished = time.Now().UTC()
	}

	var held []iface

	rec, err := r.store.update(k.id, func(rec *record) {
		held = rec.Interfaces

		rec.Status = task.StatusExited
		if lost {
			rec.Status = task.StatusDead
		}

		rec.ExitCode = ended.ExitCode
		rec.FinishedAt = finished
		rec.Interfaces = nil
	})
	if err != nil && !errors.Is(err, errNoMachine) {
		r.logger.Error("what a task ended with could not be kept", "machine", k.id, "error", err)
	}

	// a machine turns itself off, which puts its disks away as they should
	// be; one that does not in time is ended from outside.
	if !lost {
		asked, cancelAsking := context.WithTimeout(ctx, settleTimeout)
		if err := k.client.PowerOff(asked); err == nil {
			r.waitGone(ctx, k.id)
		}
		cancelAsking()
	}

	if err := r.launcher.Terminate(ctx, k.id); err != nil {
		r.logger.Error("a machine could not be let go", "machine", k.id, "error", err)
	}

	if err := r.networks.release(k.id); err != nil {
		r.logger.Error("what a machine held could not be given back", "machine", k.id, "error", err)
	}

	r.detach(k.id)
	k.close()

	r.refreshHosts(ctx, held)

	r.logger.Info("machine stopped", "machine", k.id, "exit_code", ended.ExitCode, "lost", lost)

	if err == nil && rec.Execution.AutoRemove {
		go func() {
			if err := r.Delete(context.WithoutCancel(ctx), k.id); err != nil {
				r.logger.Error("a machine that removes itself could not be removed", "machine", k.id, "error", err)
			}
		}()
	}
}

// alive reports whether a machine's process still runs, which is how a machine
// that stopped answering is told from one that is gone.
func (r *Runtime) alive(ctx context.Context, id string) bool {
	held, err := r.launcher.Machines(ctx, r.config.Owner)
	if err != nil {
		// the launcher not answering says nothing about the machine.
		return true
	}

	for _, m := range held {
		if m.ID == id {
			return m.Running
		}
	}

	return false
}

// waitGone waits a moment for a machine that was asked to turn off to be off.
func (r *Runtime) waitGone(ctx context.Context, id string) {
	deadline := time.Now().Add(settleTimeout)

	for time.Now().Before(deadline) {
		if !r.alive(ctx, id) {
			return
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// reconcileUntilDone takes back the machines still running from the last time
// the orchestrator ran, trying again until the launcher answers.
func (r *Runtime) reconcileUntilDone() {
	wait := retryInterval

	for {
		err := r.reconcile(r.ctx)
		if err == nil || r.ctx.Err() != nil {
			return
		}

		r.logger.Warn("the machines left running could not be taken back yet", "error", err)

		select {
		case <-time.After(wait):
		case <-r.ctx.Done():
			return
		}

		wait = min(wait*2, longestReconcileWait)
	}
}

// reconcile holds what the records say against what the host holds. A machine
// still running is looked after again; one that went while nobody was looking
// ended with it, and is started again if its restart policy says so; and a
// machine the host holds that nothing here runs any more is let go.
func (r *Runtime) reconcile(ctx context.Context) error {
	held, err := r.launcher.Machines(ctx, r.config.Owner)
	if err != nil {
		return err
	}

	alive := make(map[string]machine.Machine, len(held))
	for _, m := range held {
		if m.Running {
			alive[m.ID] = m
		}
	}

	running := make(map[string]bool)

	var again []string

	for _, rec := range r.store.all() {
		if !rec.running() {
			continue
		}

		id := rec.Execution.ID

		if launched, found := alive[id]; found && r.keeper(id) == nil {
			out, err := openOutput(filepath.Join(r.store.machineDir(id), outputName), outputLimit)
			if err != nil {
				return err
			}

			r.attach(rec, guest.NewClient(filepath.Join(launched.Root, vsockName)), out)
			running[id] = true

			continue
		}

		if r.keeper(id) != nil {
			running[id] = true

			continue
		}

		_, _ = r.store.update(id, func(rec *record) {
			rec.Status = task.StatusDead
			rec.ExitCode = lostExitCode
			rec.FinishedAt = time.Now().UTC()
			rec.Interfaces = nil
		})

		if parsePolicy(rec.Execution.RestartPolicy).restarts(lostExitCode, rec.RestartCount, rec.Stopped) {
			again = append(again, id)
		}
	}

	for _, m := range held {
		if !running[m.ID] {
			if err := r.launcher.Terminate(ctx, m.ID); err != nil {
				r.logger.Warn("a machine nothing runs any more could not be let go", "machine", m.ID, "error", err)
			}
		}
	}

	if err := r.networks.retain(running); err != nil {
		return err
	}

	for _, id := range again {
		if err := r.Start(ctx, id); err != nil {
			r.logger.Error("a machine that went away could not be started again", "machine", id, "error", err)
		}
	}

	return nil
}
