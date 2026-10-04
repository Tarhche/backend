package runs_test

import (
	"context"
	"errors"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// execProcess is the process an exec started, found by its marker.
func (h *harness) execProcess(runID, execID string) *fakes.FakeProcess {
	h.t.Helper()

	for _, process := range h.fake.Processes(runs.SandboxName(runID)) {
		if slices.Contains(process.Command().Env, "WORKLOAD_TERMINAL_SESSION="+execID) {
			return process
		}
	}

	h.t.Fatalf("run %s has no exec %s", runID, execID)

	return nil
}

func TestExec(t *testing.T) {
	t.Parallel()

	t.Run("a command runs with a terminal, a standard input and its marker, and hands its output on", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{
			Command: []string{"/bin/sh"},
			TTY:     true,
			Env:     []string{"TERM=xterm"},
			WorkDir: "/tmp",
			Rows:    50,
			Cols:    132,
		})
		require.NoError(t, err)

		process := h.execProcess(run.ID, exec.ID())

		assert.Equal(t, runs.Command{
			Argv:    []string{"/bin/sh"},
			Env:     []string{"TERM=xterm", "WORKLOAD_TERMINAL_SESSION=" + exec.ID()},
			Workdir: "/tmp",
			TTY:     true,
			Stdin:   true,
			Rows:    50,
			Cols:    132,
		}, process.Command())

		assert.Equal(t, [][2]uint16{{50, 132}}, process.Resizes(), "microsandbox starts a terminal at 24 by 80 whatever it was asked")

		_, err = exec.Write([]byte("echo hi\n"))
		require.NoError(t, err)

		require.NoError(t, exec.Resize(context.Background(), 10, 20))
		require.NoError(t, exec.CloseStdin())

		process.Write("hi\r\n")
		process.WriteErr("warning\n")
		process.Exit(3)

		var outputs []runs.Output
		for output := range exec.Output() {
			outputs = append(outputs, output)
		}

		assert.Equal(t, []runs.Output{
			{Stream: api.OutputStdout, Data: []byte("hi\r\n")},
			{Stream: api.OutputStderr, Data: []byte("warning\n")},
		}, outputs)

		assert.Equal(t, 3, exec.ExitCode())

		stdin, closed := process.StdinData()
		assert.Equal(t, "echo hi\n", stdin)
		assert.True(t, closed)
		assert.Equal(t, [][2]uint16{{50, 132}, {10, 20}}, process.Resizes())
	})

	t.Run("letting go of the output does not end the command", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"top"}})
		require.NoError(t, err)

		process := h.execProcess(run.ID, exec.ID())

		exec.Detach()
		process.Write("still writing\n")

		time.Sleep(20 * time.Millisecond)

		assert.False(t, process.Ended())
		assert.False(t, process.Closed(), "closing a command's handle would end it")

		process.Exit(0)

		select {
		case <-exec.Done():
		case <-time.After(eventually):
			t.Fatal("a command nobody reads never ended")
		}
	})

	t.Run("a signal ends a command as 128 and the signal", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"sleep", "100"}})
		require.NoError(t, err)

		require.NoError(t, exec.Signal(context.Background(), 2))

		assert.Equal(t, 130, exec.ExitCode())
	})

	t.Run("a command dies with its run's VM", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"sh"}})
		require.NoError(t, err)

		_, err = h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, 137, exec.ExitCode())
	})

	t.Run("a command that cannot start is invalid", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.SpawnFails("bash", "ENOENT")

		run := h.started(spec("web"))

		_, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"bash"}})

		require.Error(t, err)
		assert.Equal(t, api.CodeInvalid, runs.Code(err))
		assert.Contains(t, err.Error(), "ENOENT")
	})

	t.Run("a run that is not running has nothing to run a command in", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		_, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"sh"}})
		assert.Equal(t, api.CodeNotRunning, runs.Code(err))

		_, err = h.supervisor.Exec(context.Background(), "nope", api.ExecRequest{Command: []string{"sh"}})
		assert.Equal(t, api.CodeNotFound, runs.Code(err))

		_, err = h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{})
		assert.Equal(t, api.CodeInvalid, runs.Code(err))
	})
}

func TestEndExec(t *testing.T) {
	t.Parallel()

	t.Run("a command that has ended is left alone", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"sh"}})
		require.NoError(t, err)

		h.execProcess(run.ID, exec.ID()).Exit(0)

		require.NoError(t, h.supervisor.EndExec(context.Background(), run.ID, exec.ID()))

		assert.Empty(t, h.fake.Sweeps())

		err = h.supervisor.EndExec(context.Background(), run.ID, exec.ID())
		assert.Equal(t, api.CodeNotFound, runs.Code(err), "an exec that was ended is forgotten")
	})

	t.Run("a command is given a moment, then SIGTERM, swept across what it started", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"sh"}})
		require.NoError(t, err)

		process := h.execProcess(run.ID, exec.ID())

		began := time.Now()

		require.NoError(t, h.supervisor.EndExec(context.Background(), run.ID, exec.ID()))

		assert.GreaterOrEqual(t, time.Since(began), h.config.ExecEndGrace)
		assert.Equal(t, []syscall.Signal{15}, process.Signals()[:1])
		assert.Equal(t, []fakes.Sweep{{Sandbox: runs.SandboxName(run.ID), Exec: exec.ID(), Signal: 15}}, h.fake.Sweeps())
		assert.Equal(t, 143, exec.ExitCode())
	})

	t.Run("a command that will not stop is killed", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) {
			if p.Command().Argv[0] == "stubborn" {
				p.Ignore(15)
			}
		})

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"stubborn"}})
		require.NoError(t, err)

		require.NoError(t, h.supervisor.EndExec(context.Background(), run.ID, exec.ID()))

		assert.Equal(t, 137, exec.ExitCode())
		assert.Equal(t, []fakes.Sweep{
			{Sandbox: runs.SandboxName(run.ID), Exec: exec.ID(), Signal: 15},
			{Sandbox: runs.SandboxName(run.ID), Exec: exec.ID(), Signal: 9},
		}, h.fake.Sweeps())
	})

	t.Run("a command whose own process group does not hold it is ended by its handle", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) {
			if p.Command().Argv[0] == "deaf" {
				p.Deafen()
			}
		})

		run := h.started(spec("web"))

		exec, err := h.supervisor.Exec(context.Background(), run.ID, api.ExecRequest{Command: []string{"deaf"}})
		require.NoError(t, err)

		require.NoError(t, h.supervisor.EndExec(context.Background(), run.ID, exec.ID()))

		assert.True(t, h.execProcess(run.ID, exec.ID()).Closed())

		select {
		case <-exec.Done():
		case <-time.After(eventually):
			t.Fatal("the command was never ended")
		}
	})

	t.Run("an exec that is not there is not found", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		err := h.supervisor.EndExec(context.Background(), run.ID, "nope")
		assert.Equal(t, api.CodeNotFound, runs.Code(err))

		err = h.supervisor.EndExec(context.Background(), "nope", "nope")
		assert.Equal(t, api.CodeNotFound, runs.Code(err))
	})
}

func TestStats(t *testing.T) {
	t.Parallel()

	t.Run("a running run's VM's metrics, as the contract reports them", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		h.fake.SetMetrics(runs.SandboxName(run.ID), runs.Metrics{
			CPUPercent:  12.5,
			MemoryUsage: 40 << 20,
			MemoryLimit: 128 << 20,
			NetRx:       1, NetTx: 2, DiskRead: 3, DiskWrite: 4,
		})

		stats, err := h.supervisor.Stats(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, api.Stats{
			CPUPercent:    12.5,
			MemoryUsage:   40 << 20,
			MemoryLimit:   128 << 20,
			NetworkInput:  1,
			NetworkOutput: 2,
			BlockInput:    3,
			BlockOutput:   4,
		}, stats)
	})

	t.Run("a run that is not running has none", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		_, err := h.supervisor.Stats(context.Background(), run.ID)
		assert.Equal(t, api.CodeNotRunning, runs.Code(err))

		_, err = h.supervisor.Stats(context.Background(), "nope")
		assert.Equal(t, api.CodeNotFound, runs.Code(err))
	})

	t.Run("a node's stats are its running runs' summed, from one reading", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		first := h.started(spec("first"))
		second := h.started(spec("second"))
		h.started(spec("elsewhere", func(s *api.RunSpec) { s.Node = "orchestrator-2" }))
		h.create(spec("created"))

		h.fake.SetMetrics(runs.SandboxName(first.ID), runs.Metrics{CPUPercent: 10, MemoryUsage: 1, MemoryLimit: 100})
		h.fake.SetMetrics(runs.SandboxName(second.ID), runs.Metrics{CPUPercent: 5, MemoryUsage: 2, MemoryLimit: 200})

		stats, err := h.supervisor.NodeStats(context.Background(), node)
		require.NoError(t, err)

		assert.Equal(t, api.Stats{CPUPercent: 15, MemoryUsage: 3, MemoryLimit: 300}, stats)

		_, err = h.supervisor.Stats(context.Background(), first.ID)
		require.NoError(t, err)

		metrics := 0
		for _, call := range h.fake.Calls() {
			if call == "metrics" {
				metrics++
			}
		}

		assert.Equal(t, 1, metrics, "every caller within a moment shares one reading")
	})

	t.Run("a node with nothing running uses nothing, and a node is required", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		stats, err := h.supervisor.NodeStats(context.Background(), node)
		require.NoError(t, err)
		assert.Zero(t, stats)

		_, err = h.supervisor.NodeStats(context.Background(), "")
		assert.Equal(t, api.CodeInvalid, runs.Code(err))
	})

	t.Run("metrics that cannot be read are an error", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))
		h.fake.FailMetrics(errors.New("registry gone"))

		_, err := h.supervisor.Stats(context.Background(), run.ID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "registry gone")
	})
}

func TestListAndPull(t *testing.T) {
	t.Parallel()

	t.Run("a node's runs, narrowed by task or slug, oldest first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		first := h.create(spec("first"))
		second := h.create(spec("second", func(s *api.RunSpec) { s.Task.UUID = "shared"; s.Task.Slug = "shared-slug" }))
		third := h.create(spec("third", func(s *api.RunSpec) { s.Task.UUID = "shared"; s.Task.Slug = "shared-slug" }))
		h.create(spec("elsewhere", func(s *api.RunSpec) { s.Node = "orchestrator-2" }))

		all, err := h.supervisor.List(node, "", "")
		require.NoError(t, err)
		assert.Equal(t, []string{first.ID, second.ID, third.ID}, ids(all))

		byTask, err := h.supervisor.List(node, "shared", "")
		require.NoError(t, err)
		assert.Equal(t, []string{second.ID, third.ID}, ids(byTask))

		bySlug, err := h.supervisor.List(node, "", "shared-slug")
		require.NoError(t, err)
		assert.Equal(t, []string{second.ID, third.ID}, ids(bySlug))

		none, err := h.supervisor.List("orchestrator-3", "", "")
		require.NoError(t, err)
		assert.NotNil(t, none, "an empty list is a list")
		assert.Empty(t, none)

		_, err = h.supervisor.List("", "", "")
		assert.Equal(t, api.CodeInvalid, runs.Code(err))
	})

	t.Run("pulling caches an image, and pulling a cached one changes nothing", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.AddImage("busybox", runs.ImageConfig{Cmd: []string{"sh"}})

		require.NoError(t, h.supervisor.Pull(context.Background(), "busybox"))
		require.NoError(t, h.supervisor.Pull(context.Background(), "busybox"))

		assert.Equal(t, 1, h.fake.Pulls("busybox"))
	})

	t.Run("an image that cannot be pulled, or is for another architecture, is refused", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.AddImage("arm", runs.ImageConfig{Architecture: "arm64"})

		err := h.supervisor.Pull(context.Background(), "nope")
		assert.Equal(t, api.CodePullFailed, runs.Code(err))

		err = h.supervisor.Pull(context.Background(), "arm")
		assert.Equal(t, api.CodeNotSupported, runs.Code(err))

		err = h.supervisor.Pull(context.Background(), "")
		assert.Equal(t, api.CodeInvalid, runs.Code(err))
	})
}

func ids(list []api.Run) []string {
	var ids []string

	for _, run := range list {
		ids = append(ids, run.ID)
	}

	return ids
}
