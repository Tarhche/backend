package runs_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	// node is the orchestrator every test run belongs to, unless it says
	// otherwise.
	node = "orchestrator-1"

	// image is cached in every harness, with an entrypoint and a command of
	// its own.
	image = "nginx:alpine"

	// eventually bounds waiting for what happens in the background.
	eventually = 3 * time.Second
	tick       = 2 * time.Millisecond
)

// fakesProcess is a command running in a fake sandbox.
type fakesProcess = fakes.FakeProcess

// imageConfig is the configuration of image.
var imageConfig = runs.ImageConfig{
	Entrypoint:   []string{"/docker-entrypoint.sh"},
	Cmd:          []string{"nginx", "-g", "daemon off;"},
	StopSignal:   "SIGQUIT",
	Architecture: "amd64",
}

// harness is a supervisor over a fake microsandbox and stores in memory, with
// timings in milliseconds rather than seconds.
type harness struct {
	t *testing.T

	fake    *fakes.FakeSandboxes
	records *fakes.MemoryRecords
	journal *fakes.MemoryJournal
	ports   *fakes.MemoryHostPorts
	config  runs.Config

	supervisor *runs.Supervisor
}

type option func(*harness)

func withConfig(change func(*runs.Config)) option {
	return func(h *harness) { change(&h.config) }
}

func withRecords(records ...runs.Record) option {
	return func(h *harness) { h.records = fakes.NewMemoryRecords(records...) }
}

func withFake(fake *fakes.FakeSandboxes) option {
	return func(h *harness) { h.fake = fake }
}

func testConfig() runs.Config {
	config := runs.DefaultConfig()

	config.Budget = 4 << 30
	config.BindAddress = "10.89.0.10"
	config.StopGrace = 300 * time.Millisecond
	config.KillGrace = 100 * time.Millisecond
	config.VMStopTimeout = 50 * time.Millisecond
	config.Backoff = runs.Backoff{Initial: 5 * time.Millisecond, Max: 20 * time.Millisecond, ResetAfter: 10 * time.Second}
	config.ExecEndGrace = 50 * time.Millisecond
	config.ExecKillGrace = 50 * time.Millisecond
	config.MetricsTTL = time.Hour
	config.PullTimeout = 2 * time.Second
	config.QueueTimeout = 2 * time.Second
	config.BootTimeout = 2 * time.Second
	config.CallTimeout = 2 * time.Second
	config.RetryInterval = 5 * time.Millisecond
	config.RetryMaxInterval = 20 * time.Millisecond
	config.ServiceVersion = "test"

	return config
}

// newHarness is a supervisor that has been opened, and is shut down when the
// test ends.
func newHarness(t *testing.T, options ...option) *harness {
	t.Helper()

	h := unopened(t, options...)
	h.open()

	return h
}

// unopened is a supervisor that has not been opened yet.
func unopened(t *testing.T, options ...option) *harness {
	t.Helper()

	h := &harness{
		t:       t,
		fake:    fakes.NewFakeSandboxes(),
		records: fakes.NewMemoryRecords(),
		journal: fakes.NewMemoryJournal(),
		ports:   fakes.NewMemoryHostPorts(20000, 20999),
		config:  testConfig(),
	}

	for _, option := range options {
		option(h)
	}

	h.fake.CacheImage(image, imageConfig)

	h.supervisor = runs.New(h.fake, h.records, h.journal, h.ports, h.config, slog.New(slog.DiscardHandler))

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_ = h.supervisor.Shutdown(ctx)
	})

	return h
}

func (h *harness) open() {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(h.t, h.supervisor.Open(ctx))
}

// spec is a run of image on node, which changes as each change says.
func spec(name string, changes ...func(*api.RunSpec)) api.RunSpec {
	spec := api.RunSpec{
		Node:    node,
		Name:    name,
		Image:   image,
		CPU:     0.5,
		Memory:  128 << 20,
		Disk:    256 << 20,
		Network: api.NetworkIsolated,
		Task: api.Task{
			UUID: "task-" + name,
			Name: name,
			Slug: name + "-slug",
			Kind: "service",
		},
	}

	for _, change := range changes {
		change(&spec)
	}

	return spec
}

// create creates a run, which has to succeed.
func (h *harness) create(spec api.RunSpec) api.Run {
	h.t.Helper()

	run, err := h.supervisor.Create(context.Background(), spec)
	require.NoError(h.t, err)

	return run
}

// started creates a run and starts it, which both have to succeed.
func (h *harness) started(spec api.RunSpec) api.Run {
	h.t.Helper()

	run := h.create(spec)

	run, err := h.supervisor.Start(context.Background(), run.ID)
	require.NoError(h.t, err)
	require.Equal(h.t, api.StateRunning, run.State)

	return run
}

// main is a run's main process of a boot.
func (h *harness) main(id string, boot int) *fakes.FakeProcess {
	h.t.Helper()

	var process *fakes.FakeProcess

	require.Eventually(h.t, func() bool {
		var found bool
		process, found = h.fake.Main(runs.SandboxName(id), boot)

		return found
	}, eventually, tick, "boot %d of run %s has no main process", boot, id)

	return process
}

// get is a run, which has to be there.
func (h *harness) get(id string) api.Run {
	h.t.Helper()

	run, err := h.supervisor.Get(id)
	require.NoError(h.t, err)

	return run
}

// waitFor waits until a run is in a state, and is the run then.
func (h *harness) waitFor(id string, state api.State) api.Run {
	h.t.Helper()

	var run api.Run

	require.Eventually(h.t, func() bool {
		var err error
		run, err = h.supervisor.Get(id)

		return err == nil && run.State == state
	}, eventually, tick, "run %s never got to %s", id, state)

	return run
}

// saved is a run's record as it was last saved, once it is in a state.
func (h *harness) saved(id string, state api.State) runs.Record {
	h.t.Helper()

	var record runs.Record

	require.Eventually(h.t, func() bool {
		var found bool
		record, found = h.records.Get(id)

		return found && record.State == state
	}, eventually, tick, "run %s was never saved as %s", id, state)

	return record
}
