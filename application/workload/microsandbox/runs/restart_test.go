package runs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

func withPolicy(policy string) func(*api.RunSpec) {
	return func(s *api.RunSpec) { s.RestartPolicy = policy }
}

func TestRestartPolicies(t *testing.T) {
	t.Parallel()

	t.Run("no leaves a run that ended exited", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("no")))
		h.main(run.ID, 1).Exit(1)

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Equal(t, 1, exited.ExitCode)
		assert.Zero(t, exited.RestartCount)

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, h.fake.Boots(runs.SandboxName(run.ID)))
	})

	t.Run("on-failure restarts a run that failed", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("on-failure")))
		h.main(run.ID, 1).Exit(1)

		second := h.main(run.ID, 2)
		running := h.waitFor(run.ID, api.StateRunning)

		assert.Equal(t, uint(1), running.RestartCount)
		assert.Equal(t, run.Endpoints, running.Endpoints)

		second.Exit(0)

		exited := h.waitFor(run.ID, api.StateExited)
		assert.Equal(t, 0, exited.ExitCode, "a run that succeeded is not restarted")
		assert.Equal(t, uint(1), exited.RestartCount)
	})

	t.Run("on-failure restarts a run that was killed by a signal nobody sent", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("on-failure")))
		h.main(run.ID, 1).Die()

		h.main(run.ID, 2)
		assert.Equal(t, uint(1), h.waitFor(run.ID, api.StateRunning).RestartCount)
	})

	t.Run("on-failure:N restarts N times at most", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("on-failure:2")))

		h.main(run.ID, 1).Exit(1)
		h.main(run.ID, 2).Exit(1)
		h.main(run.ID, 3).Exit(1)

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Equal(t, uint(2), exited.RestartCount)
		assert.Equal(t, 1, exited.ExitCode)

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 3, h.fake.Boots(runs.SandboxName(run.ID)))
	})

	for _, policy := range []string{"always", "unless-stopped"} {
		t.Run(policy+" restarts a run that succeeded", func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			run := h.started(spec("web", withPolicy(policy)))
			h.main(run.ID, 1).Exit(0)

			h.main(run.ID, 2)
			assert.Equal(t, uint(1), h.waitFor(run.ID, api.StateRunning).RestartCount)
		})
	}

	t.Run("a run whose VM was lost is restarted", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("always")))
		h.main(run.ID, 1).Lose()

		h.main(run.ID, 2)
		assert.Equal(t, api.StateRunning, h.waitFor(run.ID, api.StateRunning).State)
	})

	t.Run("a run that was stopped is not brought back, even by always", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("always")))

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, stopped.State)

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, h.fake.Boots(runs.SandboxName(run.ID)))
		assert.True(t, h.saved(run.ID, api.StateExited).StoppedByRequest)
	})

	t.Run("a run that was killed is not brought back either", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("on-failure")))

		_, err := h.supervisor.Kill(context.Background(), run.ID)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, api.StateExited, h.get(run.ID).State)
	})

	t.Run("a run waits out its backoff as restarting, and a stop calls the restart off", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) {
			c.Backoff = runs.Backoff{Initial: time.Hour, Max: time.Hour, ResetAfter: time.Hour}
		}))

		run := h.started(spec("web", withPolicy("always")))
		h.main(run.ID, 1).Exit(2)

		restarting := h.waitFor(run.ID, api.StateRestarting)

		assert.Equal(t, 2, restarting.ExitCode, "a run waiting to be restarted says how it last ended")
		assert.Empty(t, restarting.Endpoints)
		assert.False(t, h.fake.Running(runs.SandboxName(run.ID)), "its VM is stopped in between")

		started, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateRestarting, started.State, "starting a run that is restarting changes nothing, as docker's does not")

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, stopped.State)
		assert.Equal(t, 2, stopped.ExitCode)
		assert.Equal(t, 1, h.fake.Boots(runs.SandboxName(run.ID)))
	})

	t.Run("a restart asked for while restarting starts the run now", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) {
			c.Backoff = runs.Backoff{Initial: time.Hour, Max: time.Hour, ResetAfter: time.Hour}
		}))

		run := h.started(spec("web", withPolicy("always")))
		h.main(run.ID, 1).Exit(2)
		h.waitFor(run.ID, api.StateRestarting)

		restarted, err := h.supervisor.Restart(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateRunning, restarted.State)
		assert.Zero(t, restarted.RestartCount)
	})

	t.Run("a deleted run is not restarted", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) {
			c.Backoff = runs.Backoff{Initial: 30 * time.Millisecond, Max: 30 * time.Millisecond, ResetAfter: time.Hour}
		}))

		run := h.started(spec("web", withPolicy("always")))
		h.main(run.ID, 1).Exit(0)
		h.waitFor(run.ID, api.StateRestarting)

		require.NoError(t, h.supervisor.Delete(context.Background(), run.ID))

		time.Sleep(100 * time.Millisecond)
		assert.False(t, h.fake.Exists(runs.SandboxName(run.ID)))
	})

	t.Run("a restart that cannot be made leaves the run exited, saying why", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("always")))

		h.fake.FailStart(assert.AnError)
		h.main(run.ID, 1).Exit(0)

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Contains(t, exited.Error, assert.AnError.Error())
		assert.Equal(t, uint(1), exited.RestartCount)
	})

	t.Run("a restart that does not fit the budget leaves the run exited, saying why", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) {
			c.Budget = 128<<20 + runs.DefaultOverhead
			c.Backoff = runs.Backoff{Initial: 50 * time.Millisecond, Max: 50 * time.Millisecond, ResetAfter: time.Hour}
		}))

		run := h.started(spec("web", withPolicy("always")))
		h.main(run.ID, 1).Exit(0)
		h.waitFor(run.ID, api.StateRestarting)

		// the room it gave back is taken before its backoff is up.
		h.started(spec("other"))

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Contains(t, exited.Error, "memory budget")
	})

	t.Run("each restart waits twice as long as the last", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) {
			c.Backoff = runs.Backoff{Initial: 40 * time.Millisecond, Max: time.Hour, ResetAfter: time.Hour}
		}))

		run := h.started(spec("web", withPolicy("always")))

		began := time.Now()

		h.main(run.ID, 1).Exit(1)
		h.main(run.ID, 2)
		first := time.Since(began)

		began = time.Now()

		h.main(run.ID, 2).Exit(1)
		h.main(run.ID, 3)
		second := time.Since(began)

		assert.GreaterOrEqual(t, first, 40*time.Millisecond)
		assert.GreaterOrEqual(t, second, 80*time.Millisecond)
		assert.Equal(t, uint(2), h.waitFor(run.ID, api.StateRunning).RestartCount)
	})
}
