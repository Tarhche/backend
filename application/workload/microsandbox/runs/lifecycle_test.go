package runs_test

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	t.Run("a run is recorded, and nothing is booted", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{32}$`), run.ID, "an ID is a UUID v7 in hex")
		assert.Equal(t, api.StateCreated, run.State)
		assert.Equal(t, spec("web"), run.RunSpec)
		assert.False(t, run.CreatedAt.IsZero())
		assert.True(t, run.StartedAt.IsZero())
		assert.Empty(t, run.Endpoints)

		assert.False(t, h.fake.Exists(runs.SandboxName(run.ID)), "creating a run boots nothing, as docker create starts nothing")

		record, found := h.records.Get(run.ID)
		require.True(t, found)
		assert.Equal(t, api.StateCreated, record.State)
	})

	t.Run("a name its node already uses is refused", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		first := h.create(spec("web"))

		_, err := h.supervisor.Create(context.Background(), spec("web"))

		require.Error(t, err)
		assert.Equal(t, api.CodeNameInUse, runs.Code(err))
		assert.Contains(t, err.Error(), first.ID)

		other := h.create(spec("web", func(s *api.RunSpec) { s.Node = "orchestrator-2" }))
		assert.NotEqual(t, first.ID, other.ID, "names are unique on a node, not everywhere")
	})

	t.Run("a name is free again once its run is deleted", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		first := h.create(spec("web"))
		require.NoError(t, h.supervisor.Delete(context.Background(), first.ID))

		h.create(spec("web"))
	})

	t.Run("a spec that does not hold is invalid", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.supervisor.Create(context.Background(), spec("web", func(s *api.RunSpec) { s.Memory = 0 }))

		require.Error(t, err)
		assert.Equal(t, api.CodeInvalid, runs.Code(err))
		assert.Zero(t, h.records.Len())
	})

	t.Run("a spec microsandbox cannot run is not supported", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.supervisor.Create(context.Background(), spec("web", func(s *api.RunSpec) { s.Network = "none" }))

		assert.Equal(t, api.CodeNotSupported, runs.Code(err))
	})

	t.Run("a run that cannot be recorded is not created", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.records.Fail(errors.New("disk full"))

		_, err := h.supervisor.Create(context.Background(), spec("web"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk full")

		h.records.Fail(nil)
		h.create(spec("web"))
	})
}

func TestStart(t *testing.T) {
	t.Parallel()

	t.Run("the VM boots with what the run asked for, and its main process runs", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		created := h.create(spec("web", func(s *api.RunSpec) {
			s.Memory = 16 << 20
			s.CPU = 1.5
			s.Environment = []string{"A=1", "B=two"}
			s.WorkingDir = "/srv"
			s.Ports = []uint16{80, 443}
			s.Network = api.NetworkPublic
		}))

		run, err := h.supervisor.Start(context.Background(), created.ID)
		require.NoError(t, err)

		assert.Equal(t, api.StateRunning, run.State)
		assert.False(t, run.StartedAt.IsZero())
		assert.Equal(t, []api.Endpoint{{Port: 80, HostPort: 20000}, {Port: 443, HostPort: 20001}}, run.Endpoints)

		sandbox, found := h.fake.Spec(runs.SandboxName(run.ID))
		require.True(t, found)

		assert.Equal(t, runs.SandboxSpec{
			Name:   runs.SandboxName(run.ID),
			Image:  image,
			CPUs:   1.5,
			Memory: 64 << 20,
			Disk:   256 << 20,
			Env:    map[string]string{"A": "1", "B": "two"},
			Labels: map[string]string{
				runs.LabelManaged: "true",
				runs.LabelRun:     run.ID,
				runs.LabelNode:    node,
			},
			Workdir: "/srv",
			Network: "public",
			Ports: []runs.PortBinding{
				{Bind: "10.89.0.10", HostPort: 20000, GuestPort: 80},
				{Bind: "10.89.0.10", HostPort: 20001, GuestPort: 443},
			},
			Nameservers:       []string{"1.1.1.1", "9.9.9.9"},
			MaxTCPConnections: 256,
		}, sandbox, "memory is raised to the floor, and sizes and CPUs are passed as they were asked for")

		main := h.main(run.ID, 1)
		assert.Equal(t, runs.Command{Argv: []string{"/docker-entrypoint.sh", "nginx", "-g", "daemon off;"}}, main.Command(),
			"the main process has no stdin and runs in the sandbox's environment")

		record := h.saved(run.ID, api.StateRunning)
		assert.Equal(t, uint64(16<<20), record.Spec.Memory, "a run's limits are kept as they were asked for")
		assert.True(t, record.Sandbox)
		assert.Equal(t, run.Endpoints, record.HostPorts)
	})

	t.Run("an isolated guest has no nameservers at all", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))

		sandbox, _ := h.fake.Spec(runs.SandboxName(run.ID))
		assert.Empty(t, sandbox.Nameservers)
		assert.Equal(t, "isolated", sandbox.Network)
	})

	t.Run("a run that is up is left as it is", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		again, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, api.StateRunning, again.State)
		assert.Equal(t, 1, h.fake.Boots(runs.SandboxName(run.ID)))
	})

	t.Run("a run that is not there is not found", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.supervisor.Start(context.Background(), "nope")

		assert.Equal(t, api.CodeNotFound, runs.Code(err))
	})

	t.Run("an image that is not cached is pulled, once for every run that needs it", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.AddImage("busybox", runs.ImageConfig{Cmd: []string{"sh"}})

		var ids []string
		for _, name := range []string{"a", "b", "c", "d"} {
			ids = append(ids, h.create(spec(name, func(s *api.RunSpec) { s.Image = "busybox" })).ID)
		}

		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Go(func() {
				_, err := h.supervisor.Start(context.Background(), id)
				assert.NoError(t, err)
			})
		}
		wg.Wait()

		assert.LessOrEqual(t, h.fake.Pulls("busybox"), 4)
		assert.GreaterOrEqual(t, h.fake.Pulls("busybox"), 1)

		for _, id := range ids {
			assert.Equal(t, api.StateRunning, h.get(id).State)
		}
	})

	t.Run("an image that cannot be pulled leaves the run as it was", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web", func(s *api.RunSpec) { s.Image = "nope:latest" }))

		_, err := h.supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)
		assert.Equal(t, api.CodePullFailed, runs.Code(err))
		assert.Contains(t, err.Error(), "manifest unknown")
		assert.Equal(t, api.StateCreated, h.get(run.ID).State)
		assert.False(t, h.fake.Exists(runs.SandboxName(run.ID)))
	})

	t.Run("an image for another architecture is not supported", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.CacheImage("arm-only", runs.ImageConfig{Cmd: []string{"sh"}, Architecture: "arm64"})

		run := h.create(spec("web", func(s *api.RunSpec) { s.Image = "arm-only" }))

		_, err := h.supervisor.Start(context.Background(), run.ID)

		assert.Equal(t, api.CodeNotSupported, runs.Code(err))
		assert.Contains(t, err.Error(), "built for arm64")
	})

	t.Run("a VM that cannot be made leaves nothing behind, and the run as it was", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.FailCreate(errors.New("cancelled"), true)

		run := h.create(spec("web"))

		_, err := h.supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
		assert.Equal(t, api.StateCreated, h.get(run.ID).State)
		assert.False(t, h.fake.Exists(runs.SandboxName(run.ID)), "a stopped sandbox a failed create left would refuse the next create")

		h.fake.FailCreate(nil, false)

		started, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateRunning, started.State)
	})

	t.Run("a stopped VM is booted again rather than made again, on the same ports", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", func(s *api.RunSpec) { s.Ports = []uint16{80} }))

		_, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		again, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, run.Endpoints, again.Endpoints)
		assert.Equal(t, 2, h.fake.Boots(runs.SandboxName(run.ID)))
		assert.Equal(t, []string{"create " + runs.SandboxName(run.ID), "start " + runs.SandboxName(run.ID)}, startsAndCreates(h.fake.Calls()))
	})

	t.Run("a VM that went away is made again, on the same ports", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", func(s *api.RunSpec) { s.Ports = []uint16{80} }))

		_, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		require.NoError(t, h.fake.Remove(context.Background(), runs.SandboxName(run.ID)))

		again, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, run.Endpoints, again.Endpoints)
		assert.Equal(t, 1, h.fake.Boots(runs.SandboxName(run.ID)), "a new sandbox")
	})

	t.Run("a VM that is there and will not boot is a failure", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		_, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		h.fake.FailStart(errors.New("disk locked"))

		_, err = h.supervisor.Start(context.Background(), run.ID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk locked")
		assert.Equal(t, api.StateExited, h.get(run.ID).State)
	})
}

func TestMainProcess(t *testing.T) {
	t.Parallel()

	image := runs.ImageConfig{Entrypoint: []string{"/entrypoint.sh"}, Cmd: []string{"serve"}}

	tests := []struct {
		name       string
		entrypoint []string
		command    []string
		want       []string
	}{
		{"nothing asked for runs the image's entrypoint and command", nil, nil, []string{"/entrypoint.sh", "serve"}},
		{"a command alone runs under the image's entrypoint", nil, []string{"python", "-c", "print(1)"}, []string{"/entrypoint.sh", "python", "-c", "print(1)"}},
		{"an entrypoint alone drops the image's command", []string{"/bin/sh"}, nil, []string{"/bin/sh"}},
		{"both run together", []string{"/bin/sh", "-c"}, []string{"exit 3"}, []string{"/bin/sh", "-c", "exit 3"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.fake.CacheImage("app", image)

			run := h.started(spec("app", func(s *api.RunSpec) {
				s.Image = "app"
				s.Entrypoint = test.entrypoint
				s.Command = test.command
			}))

			assert.Equal(t, test.want, h.main(run.ID, 1).Command().Argv)
		})
	}

	t.Run("nothing to run is invalid, and boots nothing", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.CacheImage("empty", runs.ImageConfig{})

		run := h.create(spec("app", func(s *api.RunSpec) { s.Image = "empty" }))

		_, err := h.supervisor.Start(context.Background(), run.ID)

		assert.Equal(t, api.CodeInvalid, runs.Code(err))
		assert.Equal(t, api.StateCreated, h.get(run.ID).State)
		assert.False(t, h.fake.Exists(runs.SandboxName(run.ID)))
	})
}

func TestExitCodes(t *testing.T) {
	t.Parallel()

	t.Run("an exit with N is N, and the VM stops with it", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))

		h.main(run.ID, 1).Exit(3)

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Equal(t, 3, exited.ExitCode)
		assert.Empty(t, exited.Error)
		assert.False(t, exited.FinishedAt.IsZero())
		assert.Empty(t, exited.Endpoints)
		assert.False(t, h.fake.Running(runs.SandboxName(run.ID)), "every end of a main process stops its VM")
		assert.True(t, h.main(run.ID, 1).Closed())
	})

	t.Run("the stop signal is 128 and the signal", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, stopped.State)
		assert.Equal(t, 128+3, stopped.ExitCode, "the image's STOPSIGNAL is SIGQUIT")
		assert.Equal(t, []syscall.Signal{3}, h.main(run.ID, 1).Signals())
	})

	t.Run("SIGTERM is 143 for an image that names no stop signal", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.CacheImage("plain", runs.ImageConfig{Cmd: []string{"sh"}})

		run := h.started(spec("web", func(s *api.RunSpec) { s.Image = "plain" }))

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, 143, stopped.ExitCode)
	})

	t.Run("a process that traps the stop signal exits as it likes", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) { p.Trap(3, 0) })

		run := h.started(spec("web"))

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, 0, stopped.ExitCode)
	})

	t.Run("a process that ignores the stop signal is killed once its grace is up", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) { p.Ignore(3) })

		run := h.started(spec("web"))

		began := time.Now()

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 50*time.Millisecond)
		require.NoError(t, err)

		assert.Equal(t, 137, stopped.ExitCode)
		assert.Empty(t, stopped.Error)
		assert.GreaterOrEqual(t, time.Since(began), 50*time.Millisecond, "the grace asked for is given")
		assert.Equal(t, []syscall.Signal{3, 9}, h.main(run.ID, 1).Signals())
	})

	t.Run("SIGKILL from a kill is 137", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		killed, err := h.supervisor.Kill(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, killed.State)
		assert.Equal(t, 137, killed.ExitCode)
		assert.Equal(t, []syscall.Signal{9}, h.main(run.ID, 1).Signals(), "a kill sends no stop signal first")
	})

	t.Run("a signal nobody sent is 137, killed", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))

		h.main(run.ID, 1).Die()

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Equal(t, 137, exited.ExitCode)
		assert.Equal(t, runs.ReasonKilled, exited.Error)
	})

	t.Run("a lost VM is 137, vm_lost", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))

		h.main(run.ID, 1).Lose()

		exited := h.waitFor(run.ID, api.StateExited)

		assert.Equal(t, 137, exited.ExitCode)
		assert.Equal(t, runs.ReasonVMLost, exited.Error)
	})

	for _, test := range []struct {
		errno string
		code  int
	}{
		{"ENOENT", 127},
		{"EACCES", 126},
		{"ENOEXEC", 126},
		{"EMFILE", 1},
	} {
		t.Run("a program that never started with "+test.errno, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.fake.SpawnFails("/docker-entrypoint.sh", test.errno)

			run := h.create(spec("job"))

			_, err := h.supervisor.Start(context.Background(), run.ID)

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.errno)

			exited := h.get(run.ID)

			assert.Equal(t, api.StateExited, exited.State)
			assert.Equal(t, test.code, exited.ExitCode)
			assert.Contains(t, exited.Error, test.errno)
			assert.False(t, h.fake.Running(runs.SandboxName(run.ID)))
		})
	}

	t.Run("a process that answers no signal has its VM stopped under it", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) { p.Deafen() })

		run := h.started(spec("web"))

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 20*time.Millisecond)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, stopped.State)
		assert.Equal(t, 137, stopped.ExitCode)
		assert.Empty(t, stopped.Error, "the service stopped the VM itself, so nothing was lost")
	})

	t.Run("a new start clears how the last run ended", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))
		h.main(run.ID, 1).Die()
		h.waitFor(run.ID, api.StateExited)

		again, err := h.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Zero(t, again.ExitCode)
		assert.Empty(t, again.Error)
	})
}

func TestStopAndKill(t *testing.T) {
	t.Parallel()

	t.Run("a run that is not up is left as it is", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		stopped, err := h.supervisor.Stop(context.Background(), run.ID, 0)
		require.NoError(t, err)
		assert.Equal(t, api.StateCreated, stopped.State)

		killed, err := h.supervisor.Kill(context.Background(), run.ID)
		require.NoError(t, err)
		assert.Equal(t, api.StateCreated, killed.State)
	})

	t.Run("an exited run is left as it is", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))
		h.main(run.ID, 1).Exit(0)
		h.waitFor(run.ID, api.StateExited)

		killed, err := h.supervisor.Kill(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Equal(t, 0, killed.ExitCode)
	})

	t.Run("a kill does not wait out a stop's grace", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) { p.Ignore(3) })

		run := h.started(spec("web"))

		stopped := make(chan api.Run, 1)
		go func() {
			stop, _ := h.supervisor.Stop(context.Background(), run.ID, time.Hour)
			stopped <- stop
		}()

		h.waitFor(run.ID, api.StateStopping)

		began := time.Now()

		killed, err := h.supervisor.Kill(context.Background(), run.ID)
		require.NoError(t, err)

		assert.Less(t, time.Since(began), time.Second)
		assert.Equal(t, 137, killed.ExitCode)
		assert.Equal(t, 137, (<-stopped).ExitCode)
	})

	t.Run("a run that is not there is not found", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.supervisor.Stop(context.Background(), "nope", 0)
		assert.Equal(t, api.CodeNotFound, runs.Code(err))

		_, err = h.supervisor.Kill(context.Background(), "nope")
		assert.Equal(t, api.CodeNotFound, runs.Code(err))
	})

	t.Run("a stopping run is still reached on its ports", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.fake.OnExec(func(p *fakesProcess) { p.Ignore(3) })

		run := h.started(spec("web", func(s *api.RunSpec) { s.Ports = []uint16{80} }))

		go func() { _, _ = h.supervisor.Stop(context.Background(), run.ID, 200*time.Millisecond) }()

		stopping := h.waitFor(run.ID, api.StateStopping)

		assert.Equal(t, run.Endpoints, stopping.Endpoints)

		h.waitFor(run.ID, api.StateExited)
	})
}

func TestRestart(t *testing.T) {
	t.Parallel()

	t.Run("a restart stops the run and starts it again, and is not counted", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", func(s *api.RunSpec) { s.Ports = []uint16{80} }))

		restarted, err := h.supervisor.Restart(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateRunning, restarted.State)
		assert.Zero(t, restarted.RestartCount, "docker counts the restart policy's restarts only")
		assert.Equal(t, run.Endpoints, restarted.Endpoints)
		assert.Equal(t, 2, h.fake.Boots(runs.SandboxName(run.ID)))
		assert.Equal(t, []syscall.Signal{3}, h.main(run.ID, 1).Signals())
	})

	t.Run("a run that is not up is started", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		restarted, err := h.supervisor.Restart(context.Background(), run.ID, 0)
		require.NoError(t, err)

		assert.Equal(t, api.StateRunning, restarted.State)
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("a running run is killed and forgotten, its journal and its sandbox too", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", func(s *api.RunSpec) { s.Ports = []uint16{80} }))

		main := h.main(run.ID, 1)
		main.Write("hello\n")

		require.Eventually(t, func() bool { return h.journal.Has(run.ID) }, eventually, tick)

		require.NoError(t, h.supervisor.Delete(context.Background(), run.ID))

		assert.Equal(t, []syscall.Signal{9}, main.Signals())
		assert.False(t, h.fake.Exists(runs.SandboxName(run.ID)))
		assert.False(t, h.journal.Has(run.ID))
		assert.Equal(t, []uint16{20000}, h.ports.Released())

		_, found := h.records.Get(run.ID)
		assert.False(t, found)

		_, err := h.supervisor.Get(run.ID)
		assert.Equal(t, api.CodeNotFound, runs.Code(err))

		err = h.supervisor.Delete(context.Background(), run.ID)
		assert.Equal(t, api.CodeNotFound, runs.Code(err), "a second delete is not found, which a caller takes as done")
	})

	t.Run("a run that was never started is forgotten", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		require.NoError(t, h.supervisor.Delete(context.Background(), run.ID))

		assert.Zero(t, h.records.Len())
	})
}

func TestAdmission(t *testing.T) {
	t.Parallel()

	t.Run("a VM that would take the budget past its end is refused, and the run is left as it was", func(t *testing.T) {
		t.Parallel()

		// room for two VMs of 128 MiB, each with the overhead counted on top.
		h := newHarness(t, withConfig(func(c *runs.Config) { c.Budget = 2 * (128<<20 + runs.DefaultOverhead) }))

		first := h.started(spec("first"))
		h.started(spec("second"))

		third := h.create(spec("third"))

		_, err := h.supervisor.Start(context.Background(), third.ID)

		require.Error(t, err)
		assert.Equal(t, api.CodeCapacity, runs.Code(err))
		assert.Contains(t, err.Error(), "memory budget")
		assert.Equal(t, api.StateCreated, h.get(third.ID).State)
		assert.False(t, h.fake.Exists(runs.SandboxName(third.ID)))

		_, err = h.supervisor.Stop(context.Background(), first.ID, 0)
		require.NoError(t, err)

		started, err := h.supervisor.Start(context.Background(), third.ID)
		require.NoError(t, err, "a stopped VM gives back what it was admitted for")
		assert.Equal(t, api.StateRunning, started.State)
	})

	t.Run("memory under the floor is admitted at the floor", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) { c.Budget = runs.DefaultMemoryFloor + runs.DefaultOverhead }))

		h.started(spec("first", func(s *api.RunSpec) { s.Memory = 1 << 20 }))

		second := h.create(spec("second", func(s *api.RunSpec) { s.Memory = 1 << 20 }))

		_, err := h.supervisor.Start(context.Background(), second.ID)

		assert.Equal(t, api.CodeCapacity, runs.Code(err))
	})

	t.Run("a VM whose main process ended gives back what it was admitted for", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withConfig(func(c *runs.Config) { c.Budget = 128<<20 + runs.DefaultOverhead }))

		first := h.started(spec("first"))
		h.main(first.ID, 1).Exit(0)
		h.waitFor(first.ID, api.StateExited)

		h.started(spec("second"))
	})
}

// startsAndCreates are the calls that booted a sandbox.
func startsAndCreates(calls []string) []string {
	var booted []string

	for _, call := range calls {
		if regexp.MustCompile(`^(create|start) `).MatchString(call) {
			booted = append(booted, call)
		}
	}

	return booted
}
