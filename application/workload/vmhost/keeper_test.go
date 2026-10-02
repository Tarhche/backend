package vmhost

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
)

// exits is a task that writes what it is given and ends with code, every run.
func exits(code int, lines ...string) vmMock.FakeScript {
	return func(uint64, guest.Process) vmMock.FakeRun {
		return vmMock.FakeRun{Lines: lines, Exits: true, ExitCode: code}
	}
}

func ended(v vm.VM) bool {
	return v.State == vm.StateExited || v.State == vm.StateDead
}

func TestKeeper(t *testing.T) {
	t.Parallel()

	t.Run("a job runs, says what it wrote and what it returned, and its machine is let go", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, exits(3, "hello from a microvm", "and goodbye"))

		id := w.run(spec("job"))
		agent := w.agent(id)

		finished := w.waitFor(id, ended)

		assert.Equal(t, vm.StateExited, finished.State)
		assert.Equal(t, 3, finished.ExitCode)
		assert.False(t, finished.FinishedAt.IsZero())
		assert.Empty(t, finished.Interfaces)
		assert.Empty(t, finished.Endpoints())
		assert.Equal(t, []string{"hello from a microvm", "and goodbye"}, w.output(id))

		assert.True(t, agent.PoweredOff(), "the machine was asked to turn itself off")

		_, err := w.hypervisor.Machine(w.ctx, id)
		assert.ErrorIs(t, err, vm.ErrNotFound, "the machine is let go of")
		assert.Empty(t, w.fabric.Plugged(id), "its taps and addresses are given back")
		assert.Nil(t, w.engine.keeper(id))
	})

	t.Run("a task that fails is started again in its machine as often as its policy says", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, exits(1, "attempt"))

		s := spec("failing")
		s.RestartPolicy = "on-failure:2"

		id := w.run(s)
		finished := w.waitFor(id, ended)

		assert.Equal(t, vm.StateExited, finished.State)
		assert.Equal(t, uint(2), finished.RestartCount)
		assert.Equal(t, 1, finished.ExitCode)
		assert.Equal(t, []string{"attempt", "attempt", "attempt"}, w.output(id))
		assert.Len(t, w.hypervisor.Booted(), 1, "in the one machine")
	})

	t.Run("a task is started again after a wait that grows each time", func(t *testing.T) {
		t.Parallel()

		var (
			lock  sync.Mutex
			waits []uint
		)

		w := newWorld(t, exits(1))
		w.engine.timing.backoff = func(restarts uint) time.Duration {
			lock.Lock()
			defer lock.Unlock()

			waits = append(waits, restarts)

			return time.Millisecond
		}

		s := spec("failing")
		s.RestartPolicy = "on-failure:3"

		id := w.run(s)
		w.waitFor(id, ended)

		lock.Lock()
		defer lock.Unlock()

		assert.Equal(t, []uint{0, 1, 2}, waits, "the wait is the policy's for the restarts so far")
	})

	t.Run("a task that ends well is not started again on failure alone", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, exits(0, "done"))

		s := spec("job")
		s.RestartPolicy = "on-failure"

		finished := w.waitFor(w.run(s), ended)

		assert.Equal(t, 0, finished.ExitCode)
		assert.Zero(t, finished.RestartCount)
	})

	t.Run("a machine that goes away under its task, with nothing to start it again, is dead", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		w.hypervisor.Crash(id)

		dead := w.waitFor(id, inState(vm.StateDead))

		assert.Equal(t, lostExitCode, dead.ExitCode)
		assert.Equal(t, reasonLost, dead.Reason)
		assert.Empty(t, w.fabric.Plugged(id))
	})

	t.Run("a machine that goes away under its task is booted again when its policy says so, restarting meanwhile", func(t *testing.T) {
		t.Parallel()

		gate := make(chan struct{})

		w := newWorld(t, nil)
		hypervisor := &gatedHypervisor{FakeHypervisor: w.hypervisor}
		w.engine.hypervisor = hypervisor

		s := spec("service")
		s.RestartPolicy = "always"

		id := w.run(s)

		hypervisor.gate(gate)
		w.hypervisor.Crash(id)

		// never dead, which the orchestrator would take for a failed task.
		waiting := w.waitFor(id, inState(vm.StateRestarting))
		assert.Equal(t, lostExitCode, waiting.ExitCode)
		assert.Contains(t, waiting.Reason, reasonLost)

		close(gate)

		running := w.waitFor(id, inState(vm.StateRunning))
		assert.Equal(t, uint(1), running.RestartCount)
		assert.Empty(t, running.Reason)
		assert.Len(t, w.hypervisor.Booted(), 2, "booted again")
		assert.NotEmpty(t, w.fabric.Plugged(id))
	})

	t.Run("a vm that removes itself is deleted once its task ended", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, exits(0))

		s := spec("ephemeral")
		s.AutoRemove = true

		id := w.run(s)

		require.Eventually(t, func() bool {
			_, err := w.engine.VM(w.ctx, id)

			return errors.Is(err, vm.ErrNotFound)
		}, 5*time.Second, 5*time.Millisecond)
	})

	t.Run("a run newer than the one written down is the one looked after", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		agent := w.agent(id)

		// the agent started the task again, and nobody wrote it down: a
		// vmhost went away in between.
		agent.Exit(0)
		_, err := agent.Start(w.ctx, guest.Process{Args: []string{"sh"}})
		require.NoError(t, err)

		running := w.waitFor(id, func(v vm.VM) bool { return v.Generation == 2 })
		assert.Equal(t, vm.StateRunning, running.State)
	})
}

func TestEngine_Halt(t *testing.T) {
	t.Parallel()

	t.Run("a stopped task ends on the term it is sent, and no policy starts it again", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		s := spec("service")
		s.RestartPolicy = "always"

		id := w.run(s)
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))

		stopped := w.vm(id)
		assert.Equal(t, vm.StateExited, stopped.State)
		assert.Equal(t, 143, stopped.ExitCode)
		assert.True(t, stopped.Stopped)
		assert.Zero(t, stopped.RestartCount)

		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second), "stopping a vm that does not run is stopping nothing")

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, vm.StateExited, w.vm(id).State)
		assert.Len(t, w.hypervisor.Booted(), 1)
	})

	t.Run("a killed task returns 137, and a vm that does not run cannot be killed", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		require.NoError(t, w.engine.Kill(w.ctx, id))

		killed := w.vm(id)
		assert.Equal(t, vm.StateExited, killed.State)
		assert.Equal(t, 137, killed.ExitCode)

		assert.ErrorIs(t, w.engine.Kill(w.ctx, id), vm.ErrNotRunning)
	})

	t.Run("a task waiting to be started again is stopped where it waits", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, exits(1))
		w.engine.timing.backoff = func(uint) time.Duration { return time.Minute }

		s := spec("failing")
		s.RestartPolicy = "always"

		id := w.run(s)
		w.waitFor(id, inState(vm.StateRestarting))

		stopping := time.Now()
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))

		assert.Less(t, time.Since(stopping), 10*time.Second, "the wait was cut short")
		assert.Equal(t, vm.StateExited, w.vm(id).State)
		assert.Equal(t, 1, w.vm(id).ExitCode)
	})

	t.Run("a machine whose agent does not answer is ended from outside", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))

		// the agent is gone, and the machine's vmm still runs.
		w.agent(id).Hang()

		require.NoError(t, w.engine.Kill(w.ctx, id))

		killed := w.vm(id)
		assert.Equal(t, vm.StateExited, killed.State)
		assert.Equal(t, lostExitCode, killed.ExitCode)

		_, err := w.hypervisor.Machine(w.ctx, id)
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("a restart is never an end, to anybody watching it", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, func(uint64, guest.Process) vmMock.FakeRun {
			return vmMock.FakeRun{Lines: []string{"up"}}
		})

		id := w.run(spec("service"))
		require.Eventually(t, func() bool { return len(w.output(id)) == 1 }, 5*time.Second, 5*time.Millisecond)

		for range 2 {
			statuses := w.watchWhile(id, func() error { return w.engine.Restart(w.ctx, id) })

			for _, state := range statuses {
				assert.Contains(t, []vm.State{vm.StateRunning, vm.StateRestarting}, state)
			}
		}

		// and when it had been stopped, it boots again.
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))

		statuses := w.watchWhile(id, func() error { return w.engine.Restart(w.ctx, id) })
		for _, state := range statuses {
			assert.Contains(t, []vm.State{vm.StateExited, vm.StateRunning, vm.StateRestarting}, state)
		}

		restarted := w.vm(id)
		assert.Equal(t, vm.StateRunning, restarted.State)
		assert.False(t, restarted.Stopped)
		assert.Zero(t, restarted.RestartCount, "a restart asked for is not a policy's")
		assert.Len(t, w.hypervisor.Booted(), 4)

		// what each boot wrote is numbered after what the one before did.
		require.Eventually(t, func() bool { return len(w.output(id)) == 4 }, 5*time.Second, 5*time.Millisecond)

		var seqs []uint64
		require.NoError(t, w.engine.Logs(w.ctx, id, 0, time.Time{}, false, func(line vm.LogLine) error {
			seqs = append(seqs, line.Seq)

			return nil
		}))
		assert.Equal(t, []uint64{1, 2, 3, 4}, seqs)
	})

	t.Run("deleting a running vm ends it, and takes everything it held", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		require.NoError(t, w.engine.Delete(w.ctx, id))

		_, err := w.engine.VM(w.ctx, id)
		assert.ErrorIs(t, err, vm.ErrNotFound)

		_, err = w.hypervisor.Machine(w.ctx, id)
		assert.ErrorIs(t, err, vm.ErrNotFound)
		assert.Empty(t, w.fabric.Plugged(id))
		assert.Nil(t, w.engine.keeper(id))

		assert.ErrorIs(t, w.engine.Delete(w.ctx, id), vm.ErrNotFound)
	})
}

// watchWhile does something to a VM, and says every state it read as while
// that was done.
func (w *world) watchWhile(id string, do func() error) []vm.State {
	w.t.Helper()

	done := make(chan error, 1)
	go func() { done <- do() }()

	var states []vm.State

	for {
		if v, err := w.engine.VM(w.ctx, id); err == nil {
			states = append(states, v.State)
		}

		select {
		case err := <-done:
			require.NoError(w.t, err)

			return slices.Compact(states)
		case <-time.After(time.Millisecond):
		}
	}
}
