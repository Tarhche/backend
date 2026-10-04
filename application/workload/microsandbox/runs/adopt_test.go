package runs_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// record is a run's record as a service that went away left it.
func record(id string, state api.State, changes ...func(*runs.Record)) runs.Record {
	record := runs.Record{
		ID:        id,
		Spec:      spec("run-" + id),
		State:     state,
		CreatedAt: time.Now().UTC(),
		Sandbox:   true,
	}

	for _, change := range changes {
		change(&record)
	}

	return record
}

func withRecordPolicy(policy string) func(*runs.Record) {
	return func(r *runs.Record) { r.Spec.RestartPolicy = policy }
}

// seed puts a run's sandbox in place, as a service that went away left it.
func (h *harness) seed(id string, running bool) {
	h.fake.Seed(runs.SandboxName(id), map[string]string{
		runs.LabelManaged: "true",
		runs.LabelRun:     id,
		runs.LabelNode:    node,
	}, running)
}

func TestNotReady(t *testing.T) {
	t.Parallel()

	t.Run("nothing but Info answers before the service is opened", func(t *testing.T) {
		t.Parallel()

		h := unopened(t)

		info := h.supervisor.Info()
		assert.False(t, info.Ready)
		assert.NotEmpty(t, info.Reason)
		assert.Equal(t, api.Version, info.APIVersion)

		_, err := h.supervisor.Create(context.Background(), spec("web"))
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		_, err = h.supervisor.List(node, "", "")
		assert.Equal(t, api.CodeUnavailable, runs.Code(err), "a node that seems to hold nothing would have its tasks scheduled again")

		_, err = h.supervisor.Start(context.Background(), "any")
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		_, err = h.supervisor.Stop(context.Background(), "any", 0)
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		err = h.supervisor.Delete(context.Background(), "any")
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		err = h.supervisor.Pull(context.Background(), image)
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		assert.Error(t, h.supervisor.Ping(context.Background()))
	})

	t.Run("once opened it is ready", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		info := h.supervisor.Info()

		assert.Equal(t, api.Info{
			APIVersion:          api.Version,
			ServiceVersion:      "test",
			MicrosandboxVersion: "0.7.6",
			Ready:               true,
			Architecture:        "amd64",
		}, info)

		assert.NoError(t, h.supervisor.Ping(context.Background()))
	})

	t.Run("a runtime that cannot run anything is checked again until it can", func(t *testing.T) {
		t.Parallel()

		h := unopened(t)
		h.fake.SetCheckError(errors.New("/dev/kvm: permission denied"))

		opened := make(chan error, 1)
		go func() { opened <- h.supervisor.Open(context.Background()) }()

		require.Eventually(t, func() bool {
			return len(h.fake.Calls()) >= 2
		}, eventually, tick, "the check is tried again")

		info := h.supervisor.Info()
		assert.False(t, info.Ready)
		assert.Contains(t, info.Reason, "/dev/kvm: permission denied")

		h.fake.SetCheckError(nil)

		require.NoError(t, <-opened)
		assert.True(t, h.supervisor.Info().Ready)
	})

	t.Run("an msb of another version than the SDK's runs nothing, and pulls nothing", func(t *testing.T) {
		t.Parallel()

		h := unopened(t)
		h.fake.SetVersions(runs.Versions{SDK: "0.7.6", Runtime: "msb 0.7.5", Architecture: "amd64"})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		require.Error(t, h.supervisor.Open(ctx))

		info := h.supervisor.Info()
		assert.False(t, info.Ready)
		assert.Equal(t, "0.7.5", info.MicrosandboxVersion, "msb's version, as a version")
		assert.Contains(t, info.Reason, "one database")

		err := h.supervisor.Pull(context.Background(), image)
		assert.Equal(t, api.CodeUnavailable, runs.Code(err), "an msb of another version would break the database the two share")

		h.fake.SetVersions(fakes.FakeVersions)
		h.open()

		assert.True(t, h.supervisor.Info().Ready)
	})

	t.Run("sandboxes that cannot be listed are listed again", func(t *testing.T) {
		t.Parallel()

		h := unopened(t)
		h.fake.FailList(errors.New("database is locked"))

		opened := make(chan error, 1)
		go func() { opened <- h.supervisor.Open(context.Background()) }()

		require.Eventually(t, func() bool {
			return len(h.supervisor.Info().Reason) > 0 && h.supervisor.Info().Reason != "the runtime has not been checked yet"
		}, eventually, tick)
		assert.Contains(t, h.supervisor.Info().Reason, "database is locked")

		h.fake.FailList(nil)

		require.NoError(t, <-opened)
	})
}

func TestReadoption(t *testing.T) {
	t.Parallel()

	for _, state := range []api.State{api.StateRunning, api.StateStopping, api.StateStarting} {
		t.Run(string(state)+", with its VM still running: the VM is stopped and the run ended by the service", func(t *testing.T) {
			t.Parallel()

			h := unopened(t, withRecords(record("a1", state)))
			h.seed("a1", true)
			h.open()

			run := h.get("a1")

			assert.Equal(t, api.StateExited, run.State)
			assert.Equal(t, 137, run.ExitCode)
			assert.Equal(t, runs.ReasonServiceRestarted, run.Error)
			assert.False(t, run.FinishedAt.IsZero())
			assert.False(t, h.fake.Running(runs.SandboxName("a1")))
			assert.Equal(t, 1, h.fake.Stops(runs.SandboxName("a1")))

			assert.Equal(t, api.StateExited, h.saved("a1", api.StateExited).State)
		})
	}

	t.Run("running, with its VM crashed: the run is ended by the service", func(t *testing.T) {
		t.Parallel()

		h := unopened(t, withRecords(record("a1", api.StateRunning)))
		h.seed("a1", false)
		h.open()

		run := h.get("a1")

		assert.Equal(t, api.StateExited, run.State)
		assert.Equal(t, 137, run.ExitCode)
		assert.Equal(t, runs.ReasonServiceRestarted, run.Error)
		assert.Zero(t, h.fake.Stops(runs.SandboxName("a1")))
	})

	t.Run("a run whose sandbox has gone is kept, and given a new one on the same ports", func(t *testing.T) {
		t.Parallel()

		h := unopened(t, withRecords(record("a1", api.StateExited, func(r *runs.Record) {
			r.Spec.Ports = []uint16{80}
			r.HostPorts = []api.Endpoint{{Port: 80, HostPort: 20500}}
		})))
		h.open()

		assert.Equal(t, []uint16{20500}, h.ports.Held("a1"), "its ports are held again")

		run, err := h.supervisor.Start(context.Background(), "a1")
		require.NoError(t, err)

		assert.Equal(t, []api.Endpoint{{Port: 80, HostPort: 20500}}, run.Endpoints)

		sandbox, found := h.fake.Spec(runs.SandboxName("a1"))
		require.True(t, found)
		assert.Equal(t, []runs.PortBinding{{Bind: "10.89.0.10", HostPort: 20500, GuestPort: 80}}, sandbox.Ports)
	})

	t.Run("a sandbox that belongs to no run is destroyed", func(t *testing.T) {
		t.Parallel()

		h := unopened(t)
		h.seed("orphan", true)
		h.fake.Seed("not-ours", map[string]string{"app": "something else"}, true)
		h.open()

		assert.False(t, h.fake.Exists(runs.SandboxName("orphan")))
		assert.True(t, h.fake.Exists("not-ours"), "only sandboxes the service made are its to destroy")
	})

	t.Run("a stopped run whose sandbox is there is left as it is", func(t *testing.T) {
		t.Parallel()

		h := unopened(t, withRecords(record("a1", api.StateExited, func(r *runs.Record) { r.ExitCode = 3 })))
		h.seed("a1", false)
		h.open()

		run := h.get("a1")

		assert.Equal(t, api.StateExited, run.State)
		assert.Equal(t, 3, run.ExitCode)
		assert.Empty(t, run.Error)
	})

	for _, test := range []struct {
		policy  string
		revived bool
	}{
		{"no", false},
		{"on-failure", false},
		{"always", true},
		{"unless-stopped", true},
	} {
		t.Run("a run that was running under "+test.policy, func(t *testing.T) {
			t.Parallel()

			h := unopened(t, withRecords(record("a1", api.StateRunning, withRecordPolicy(test.policy))))
			h.seed("a1", true)
			h.open()

			run := h.get("a1")

			if !test.revived {
				assert.Equal(t, api.StateExited, run.State)

				return
			}

			assert.Equal(t, api.StateRunning, run.State, "it is running again once the service is ready")
			assert.Zero(t, run.RestartCount, "starting again after the service is no restart of the policy's")
			assert.Equal(t, 2, h.fake.Boots(runs.SandboxName("a1")))
		})
	}

	t.Run("a run the service stopped as it shut down is started again if its policy says so", func(t *testing.T) {
		t.Parallel()

		resumed := func(r *runs.Record) { r.Resume = true; r.ExitCode = 143; r.Error = runs.ReasonServiceRestarted }

		h := unopened(t, withRecords(
			record("a1", api.StateExited, withRecordPolicy("unless-stopped"), resumed),
			record("a2", api.StateExited, withRecordPolicy("no"), resumed),
		))
		h.seed("a1", false)
		h.seed("a2", false)
		h.open()

		assert.Equal(t, api.StateRunning, h.get("a1").State)
		assert.Equal(t, api.StateExited, h.get("a2").State)

		assert.False(t, h.saved("a2", api.StateExited).Resume, "a run is resumed once, if at all")
	})

	t.Run("a run that was being stopped by request is not started again", func(t *testing.T) {
		t.Parallel()

		h := unopened(t, withRecords(record("a1", api.StateStopping, withRecordPolicy("always"), func(r *runs.Record) {
			r.StoppedByRequest = true
		})))
		h.seed("a1", true)
		h.open()

		assert.Equal(t, api.StateExited, h.get("a1").State)
	})

	t.Run("a run that was waiting out its backoff keeps how it ended, and is started again", func(t *testing.T) {
		t.Parallel()

		h := unopened(t, withRecords(record("a1", api.StateRestarting, withRecordPolicy("always"), func(r *runs.Record) {
			r.ExitCode = 2
			r.RestartCount = 4
		})))
		h.seed("a1", false)
		h.open()

		run := h.get("a1")

		assert.Equal(t, api.StateRunning, run.State)
		assert.Equal(t, uint(4), run.RestartCount)
	})

	t.Run("a run that cannot be started again says why", func(t *testing.T) {
		t.Parallel()

		h := unopened(t, withRecords(record("a1", api.StateRunning, withRecordPolicy("always"))))
		h.seed("a1", true)
		h.fake.FailStart(errors.New("disk locked"))
		h.open()

		run := h.get("a1")

		assert.Equal(t, api.StateExited, run.State)
		assert.Contains(t, run.Error, "disk locked")
		assert.True(t, h.supervisor.Info().Ready, "one run that cannot start does not keep the service from being ready")
	})

	t.Run("the ports of the runs held are not handed out again", func(t *testing.T) {
		t.Parallel()

		h := unopened(t,
			withRecords(record("a1", api.StateExited, func(r *runs.Record) {
				r.Spec.Ports = []uint16{80}
				r.HostPorts = []api.Endpoint{{Port: 80, HostPort: 20000}}
			})),
		)
		h.open()

		run := h.started(spec("web", func(s *api.RunSpec) { s.Ports = []uint16{80} }))

		assert.NotEqual(t, uint16(20000), run.Endpoints[0].HostPort)
	})

	t.Run("a name a held run uses is still in use", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withRecords(record("a1", api.StateExited)))

		_, err := h.supervisor.Create(context.Background(), spec("run-a1"))

		assert.Equal(t, api.CodeNameInUse, runs.Code(err))
	})
}

func TestShutdown(t *testing.T) {
	t.Parallel()

	t.Run("every run that is up is stopped, to be resumed if its policy says so, and nothing starts", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		running := h.started(spec("web", withPolicy("always")))
		created := h.create(spec("later"))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, h.supervisor.Shutdown(ctx))

		stopped := h.get(running.ID)

		assert.Equal(t, api.StateExited, stopped.State)
		assert.Equal(t, 128+3, stopped.ExitCode, "it was stopped as a stop stops it")
		assert.Equal(t, runs.ReasonServiceRestarted, stopped.Error)
		assert.False(t, h.fake.Running(runs.SandboxName(running.ID)))

		record := h.saved(running.ID, api.StateExited)
		assert.True(t, record.Resume)
		assert.False(t, record.StoppedByRequest)

		info := h.supervisor.Info()
		assert.False(t, info.Ready)
		assert.Equal(t, "the service is shutting down", info.Reason)

		_, err := h.supervisor.Start(context.Background(), created.ID)
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		_, err = h.supervisor.Create(context.Background(), spec("another"))
		assert.Equal(t, api.CodeUnavailable, runs.Code(err))

		run, err := h.supervisor.Get(running.ID)
		require.NoError(t, err, "what it holds can still be read")
		assert.Equal(t, running.ID, run.ID)
	})

	t.Run("a run waiting out its backoff is not restarted, and is resumed later", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) {
			c.Backoff = runs.Backoff{Initial: time.Hour, Max: time.Hour, ResetAfter: time.Hour}
		}))

		run := h.started(spec("web", withPolicy("always")))
		h.main(run.ID, 1).Exit(1)
		h.waitFor(run.ID, api.StateRestarting)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, h.supervisor.Shutdown(ctx))

		record := h.saved(run.ID, api.StateExited)
		assert.True(t, record.Resume)
		assert.Equal(t, 1, record.ExitCode)
	})

	t.Run("the service that comes back resumes what the one that went away stopped", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		always := h.started(spec("always", withPolicy("always")))
		no := h.started(spec("no", withPolicy("no")))
		h.main(always.ID, 1).Write("before\n")

		require.Eventually(t, func() bool { return len(h.journal.Lines(always.ID)) == 1 }, eventually, tick)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, h.supervisor.Shutdown(ctx))

		// the same disk and the same microsandbox, under a service that
		// starts again.
		again := runs.New(h.fake, h.records, h.journal, h.ports, h.config, slog.New(slog.DiscardHandler))
		t.Cleanup(func() { _ = again.Shutdown(context.Background()) })

		require.NoError(t, again.Open(context.Background()))

		resumed, err := again.Get(always.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateRunning, resumed.State)

		left, err := again.Get(no.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateExited, left.State)
		assert.Equal(t, runs.ReasonServiceRestarted, left.Error)

		process, found := h.fake.Main(runs.SandboxName(always.ID), 2)
		require.True(t, found)
		process.Write("after\n")

		require.Eventually(t, func() bool { return len(h.journal.Lines(always.ID)) == 2 }, eventually, tick)

		lines := h.journal.Lines(always.ID)
		assert.True(t, lines[1].At.After(lines[0].At), "a line written after the service came back comes after every line written before")
		assert.Equal(t, "after", lines[1].Content)
	})
}
