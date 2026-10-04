package runs_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// mocked is a supervisor over a mocked microsandbox in which every run's VM
// boots, and whose main process is the one given.
func mocked(t *testing.T, process runs.Process, execErr error, change func(*runs.Config)) (*runs.Supervisor, *fakes.MockSandboxes, *fakes.MemoryJournal) {
	t.Helper()

	sandbox := &fakes.MockSandbox{}
	sandbox.On("Exec", mock.Anything, mock.Anything).Return(process, execErr)
	sandbox.On("Close").Return(nil)

	sandboxes := &fakes.MockSandboxes{}
	sandboxes.On("Check", mock.Anything).Return(fakes.FakeVersions, nil)
	sandboxes.On("List", mock.Anything, mock.Anything).Return([]runs.SandboxInfo(nil), nil)
	sandboxes.On("Image", mock.Anything, image).Return(imageConfig, true, nil)
	sandboxes.On("Create", mock.Anything, mock.Anything).Return(sandbox, nil)
	sandboxes.On("Stop", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	config := testConfig()
	if change != nil {
		change(&config)
	}

	journal := fakes.NewMemoryJournal()

	supervisor := runs.New(sandboxes, fakes.NewMemoryRecords(), journal, fakes.NewMemoryHostPorts(20000, 20999), config, slog.New(slog.DiscardHandler))
	require.NoError(t, supervisor.Open(context.Background()))

	t.Cleanup(func() { _ = supervisor.Shutdown(context.Background()) })

	return supervisor, sandboxes, journal
}

// scripted is a process whose events are the ones given, and whose handle
// can be closed.
func scripted(events ...runs.Event) *fakes.MockProcess {
	channel := make(chan runs.Event, len(events))
	for _, event := range events {
		channel <- event
	}

	close(channel)

	process := &fakes.MockProcess{}
	process.On("Events").Return((<-chan runs.Event)(channel))
	process.On("Close").Return(nil)

	return process
}

func TestLaunch(t *testing.T) {
	t.Parallel()

	t.Run("a main process that cannot be asked for stops the VM and leaves the run as it was", func(t *testing.T) {
		t.Parallel()

		supervisor, sandboxes, _ := mocked(t, nil, errors.New("the agent did not answer"), nil)

		run, err := supervisor.Create(context.Background(), spec("web"))
		require.NoError(t, err)

		_, err = supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "the agent did not answer")

		got, err := supervisor.Get(run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateCreated, got.State)

		sandboxes.AssertCalled(t, "Stop", mock.Anything, runs.SandboxName(run.ID), mock.Anything)
	})

	t.Run("a VM lost before its main process started ended the run", func(t *testing.T) {
		t.Parallel()

		process := scripted(runs.Event{Kind: runs.EventLost})

		supervisor, _, _ := mocked(t, process, nil, nil)

		run, err := supervisor.Create(context.Background(), spec("web"))
		require.NoError(t, err)

		_, err = supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "went away")

		got, err := supervisor.Get(run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateExited, got.State)
		assert.Equal(t, 137, got.ExitCode)
		assert.Equal(t, runs.ReasonVMLost, got.Error)
	})

	t.Run("a stream that closed before its process started is a lost VM too", func(t *testing.T) {
		t.Parallel()

		supervisor, _, _ := mocked(t, scripted(), nil, nil)

		run, err := supervisor.Create(context.Background(), spec("web"))
		require.NoError(t, err)

		_, err = supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)

		got, err := supervisor.Get(run.ID)
		require.NoError(t, err)
		assert.Equal(t, runs.ReasonVMLost, got.Error)
	})

	t.Run("output before the start was announced means it started", func(t *testing.T) {
		t.Parallel()

		process := scripted(
			runs.Event{Kind: runs.EventStdout, Data: []byte("early\n")},
			runs.Event{Kind: runs.EventExited, ExitCode: 4},
		)

		supervisor, _, journal := mocked(t, process, nil, nil)

		run, err := supervisor.Create(context.Background(), spec("web"))
		require.NoError(t, err)

		started, err := supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateRunning, started.State)

		require.Eventually(t, func() bool {
			got, err := supervisor.Get(run.ID)

			return err == nil && got.State == api.StateExited && got.ExitCode == 4
		}, eventually, tick)

		lines := journal.Lines(run.ID)
		require.Len(t, lines, 1)
		assert.Equal(t, "early", lines[0].Content)
	})

	t.Run("an exit before the start was announced is an exit", func(t *testing.T) {
		t.Parallel()

		supervisor, _, _ := mocked(t, scripted(runs.Event{Kind: runs.EventExited, ExitCode: 0}), nil, nil)

		run, err := supervisor.Create(context.Background(), spec("job"))
		require.NoError(t, err)

		_, err = supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			got, err := supervisor.Get(run.ID)

			return err == nil && got.State == api.StateExited
		}, eventually, tick)
	})

	t.Run("a main process that never says it started is given up on, and ended", func(t *testing.T) {
		t.Parallel()

		silent := make(chan runs.Event)

		process := &fakes.MockProcess{}
		process.On("Events").Return((<-chan runs.Event)(silent))
		process.On("Close").Return(nil).Once()

		supervisor, sandboxes, _ := mocked(t, process, nil, func(c *runs.Config) { c.BootTimeout = 50 * time.Millisecond })

		run, err := supervisor.Create(context.Background(), spec("web"))
		require.NoError(t, err)

		_, err = supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "did not start")

		process.AssertCalled(t, "Close")
		sandboxes.AssertCalled(t, "Stop", mock.Anything, runs.SandboxName(run.ID), mock.Anything)

		got, err := supervisor.Get(run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateCreated, got.State)
	})

	t.Run("a run carries on when its record cannot be saved", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))
		h.records.Fail(errors.New("disk full"))

		started, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, api.StateRunning, started.State)

		h.records.Fail(nil)

		_, err = h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, h.saved(run.ID, api.StateExited).State, "the next save writes the run as it is")
	})
}

func TestCode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, api.CodeInternal, runs.Code(errors.New("anything")))
	assert.Equal(t, api.CodeCapacity, runs.Code(&api.Error{Code: api.CodeCapacity}))
}
