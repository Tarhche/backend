package vmruntime

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

func snippet(change func(*task.Execution)) *task.Execution {
	execution := &task.Execution{
		Name:             "snippet-abcde",
		TaskUUID:         "task-uuid",
		TaskName:         "a-request-id",
		Slug:             "snippet-abcde",
		Kind:             task.KindJob,
		NodeName:         nodeName,
		OwnerUUID:        "guest",
		Attempt:          1,
		Interactive:      true,
		TTL:              2 * time.Minute,
		Image:            "ghcr.io/tarhche/code-runner:go-1.24",
		ResourceLimits:   task.ResourceLimits{Cpu: 0.5, Memory: 200 << 20, Disk: 100 << 20},
		WorkingDirectory: "/code",
		ExposedPorts:     port.PortSet{8080: {}, 3000: {}},
		PortBindings:     port.PortMap{3000: {{HostIP: "0.0.0.0"}}},
		NetworkPolicy:    network.PolicyIsolated,
		Environment:      []string{"A=1"},
		Entrypoint:       []string{"/runner"},
		Command:          []string{"--timeout", "30", "fmt.Println(1)"},
	}

	if change != nil {
		change(execution)
	}

	return execution
}

func runtimeOn(e *memory.Engine) *Runtime {
	r := New(e, slog.New(slog.DiscardHandler))
	r.poll = 5 * time.Millisecond

	return r
}

func TestRuntime_Create(t *testing.T) {
	t.Parallel()

	t.Run("a run is a VM of its own, given what the task asked for", func(t *testing.T) {
		t.Parallel()

		e := memory.New(memory.WithHost("vmhost-01"))

		id, err := runtimeOn(e).Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		spec, err := e.Spec(id)
		require.NoError(t, err)

		assert.Equal(t, vm.Spec{
			ID:    id,
			Kind:  vm.KindMachine,
			Image: "ghcr.io/tarhche/code-runner:go-1.24",

			// whole vCPUs, rounded up; bytes as they were asked for.
			Resources: vm.Resources{CPUs: 1, Memory: 200 << 20, Disk: 100 << 20},

			Ports:   []port.Port{3000, 8080},
			Network: vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			Labels: map[string]string{
				vm.LabelPurpose:             vm.PurposeTask,
				vm.LabelTask:                "task-uuid",
				vm.LabelSlug:                "snippet-abcde",
				vm.LabelOwner:               "guest",
				"workload.task.name":        "a-request-id",
				"workload.task.kind":        "job",
				"workload.task.node":        nodeName,
				"workload.task.attempt":     "1",
				"workload.task.interactive": "true",
				"workload.task.ttl":         "120",
				"workload.task.image":       "ghcr.io/tarhche/code-runner:go-1.24",
			},

			// the entrypoint and the command are the process.
			Command:    []string{"/runner", "--timeout", "30", "fmt.Println(1)"},
			Env:        []string{"A=1"},
			WorkingDir: "/code",
		}, spec)
	})

	for _, tt := range []struct {
		policy network.Policy
		want   vm.Network
	}{
		{policy: network.PolicyNone, want: vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny}},
		{policy: network.PolicyIsolated, want: vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}},
		{policy: network.PolicyPublic, want: vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}},
	} {
		t.Run("a "+string(tt.policy)+" task's VM is given the network its policy maps to", func(t *testing.T) {
			t.Parallel()

			e := memory.New()

			id, err := runtimeOn(e).Create(t.Context(), snippet(func(execution *task.Execution) {
				execution.NetworkPolicy = tt.policy
			}))
			require.NoError(t, err)

			spec, err := e.Spec(id)
			require.NoError(t, err)
			assert.Equal(t, tt.want, spec.Network)
		})
	}

	t.Run("a run asked for twice is the same run, refused the second time", func(t *testing.T) {
		t.Parallel()

		r := runtimeOn(memory.New())

		first, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		_, err = r.Create(t.Context(), snippet(nil))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)

		retried, err := r.Create(t.Context(), snippet(func(execution *task.Execution) { execution.Attempt = 2 }))
		require.NoError(t, err)
		assert.NotEqual(t, first, retried, "another attempt is another run")
	})

	t.Run("cores are rounded up to whole vCPUs, and never to none", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, uint(1), cpus(0))
		assert.Equal(t, uint(1), cpus(0.25))
		assert.Equal(t, uint(2), cpus(1.2))
		assert.Equal(t, uint(2), cpus(2))
	})
}

func TestRuntime_Lookups(t *testing.T) {
	t.Parallel()

	e := memory.New()
	r := runtimeOn(e)

	_, err := r.Create(t.Context(), snippet(nil))
	require.NoError(t, err)

	_, err = r.Create(t.Context(), snippet(func(execution *task.Execution) {
		execution.TaskUUID = "other-task"
		execution.Slug = "other-fghij"
		execution.NodeName = "workload-orchestrator-02"
	}))
	require.NoError(t, err)

	// a user's VM on the same engine is no task's.
	_, err = e.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindMachine, Image: "ubuntu:24.04", Labels: map[string]string{
		vm.LabelPurpose: vm.PurposeVM, vm.LabelSlug: "snippet-abcde", vm.LabelTask: "task-uuid",
	}})
	require.NoError(t, err)

	onNode, err := r.OnNode(t.Context(), nodeName)
	require.NoError(t, err)
	require.Len(t, onNode, 1)
	assert.Equal(t, "task-uuid", onNode[0].TaskUUID)

	of, err := r.Of(t.Context(), "task-uuid")
	require.NoError(t, err)
	require.Len(t, of, 1)

	bySlug, err := r.BySlug(t.Context(), "other-fghij")
	require.NoError(t, err)
	require.Len(t, bySlug, 1)
	assert.Equal(t, "other-task", bySlug[0].TaskUUID)

	_, err = r.Inspect(t.Context(), "vm-1")
	assert.ErrorIs(t, err, domain.ErrNotExists, "a VM is not a run of a task")
}

func TestRuntime_Inspect(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		end  func(t *testing.T, e *memory.Engine, id string)

		wantStatus   task.Status
		wantExitCode int
		wantState    task.State
	}{
		{
			name:       "a run whose command is running is running",
			end:        func(*testing.T, *memory.Engine, string) {},
			wantStatus: task.StatusRunning,
			wantState:  task.Running,
		},
		{
			name:       "a job whose command succeeded has completed",
			end:        func(t *testing.T, e *memory.Engine, id string) { require.NoError(t, e.Exit(id, 0)) },
			wantStatus: task.StatusExited,
			wantState:  task.Completed,
		},
		{
			name:         "a job whose command failed has failed, with what it returned",
			end:          func(t *testing.T, e *memory.Engine, id string) { require.NoError(t, e.Exit(id, 3)) },
			wantStatus:   task.StatusExited,
			wantExitCode: 3,
			wantState:    task.Failed,
		},
		{
			name:         "a job killed without being told how was cut short rather than failed",
			end:          func(t *testing.T, e *memory.Engine, id string) { require.NoError(t, e.Exit(id, -1)) },
			wantStatus:   task.StatusExited,
			wantExitCode: 137,
			wantState:    task.Completed,
		},
		{
			name:       "a run stopped from outside has ended",
			end:        func(t *testing.T, e *memory.Engine, id string) { require.NoError(t, e.Stop(t.Context(), id)) },
			wantStatus: task.StatusExited,
			wantState:  task.Completed,
		},
		{
			name:       "a run whose VM failed has failed",
			end:        func(t *testing.T, e *memory.Engine, id string) { require.NoError(t, e.Fail(id, "the kernel panicked")) },
			wantStatus: task.StatusDead,
			wantState:  task.Failed,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			started := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
			e := memory.New(memory.WithHost("vmhost-01"), memory.WithClock(func() time.Time { return started }))
			r := runtimeOn(e)

			id, err := r.Create(t.Context(), snippet(nil))
			require.NoError(t, err)

			tt.end(t, e, id)

			execution, err := r.Inspect(t.Context(), id)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, execution.Status)
			assert.Equal(t, tt.wantExitCode, execution.ExitCode)
			assert.Equal(t, tt.wantState, task.EvaluateState(execution.Status, execution.Kind, execution.ExitCode))

			// what it is running reads back as it was written.
			assert.Equal(t, id, execution.ID)
			assert.Equal(t, "snippet-abcde", execution.Name)
			assert.Equal(t, "task-uuid", execution.TaskUUID)
			assert.Equal(t, "a-request-id", execution.TaskName)
			assert.Equal(t, task.KindJob, execution.Kind)
			assert.Equal(t, nodeName, execution.NodeName)
			assert.Equal(t, "guest", execution.OwnerUUID)
			assert.Equal(t, 1, execution.Attempt)
			assert.True(t, execution.Interactive)
			assert.Equal(t, 2*time.Minute, execution.TTL)
			assert.Equal(t, "ghcr.io/tarhche/code-runner:go-1.24", execution.Image)
			assert.Equal(t, started, execution.StartedAt)
		})
	}

	t.Run("a live snippet's ports are where its engine published them", func(t *testing.T) {
		t.Parallel()

		e := memory.New(memory.WithHost("vmhost-01"))
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		execution, err := r.Inspect(t.Context(), id)
		require.NoError(t, err)

		assert.Equal(t, port.PortMap{
			3000: {{HostIP: "vmhost-01", HostPort: 20000}},
			8080: {{HostIP: "vmhost-01", HostPort: 20001}},
		}, execution.PortBindings)
		assert.Equal(t, port.PortSet{3000: {}, 8080: {}}, execution.ExposedPorts)
	})

	t.Run("a snippet with no network publishes nothing", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(func(execution *task.Execution) {
			execution.NetworkPolicy = network.PolicyNone
		}))
		require.NoError(t, err)

		execution, err := r.Inspect(t.Context(), id)
		require.NoError(t, err)
		assert.Empty(t, execution.PortBindings)
	})
}

func TestRuntime_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("a job that ran to its end is not run again by starting it", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)
		require.NoError(t, e.Exit(id, 0))

		require.NoError(t, r.Start(t.Context(), id))

		instance, err := e.Inspect(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceExited, instance.State)
	})

	t.Run("a stopped run is booted by starting it, and a restarted one runs again", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		require.NoError(t, r.Stop(t.Context(), id))
		require.NoError(t, r.Start(t.Context(), id))

		instance, err := e.Inspect(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, instance.State)

		require.NoError(t, e.Exit(id, 1))
		require.NoError(t, r.Restart(t.Context(), id))

		instance, err = e.Inspect(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, instance.State)
	})

	t.Run("a killed run is stopped, and a deleted one is gone, twice over", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		require.NoError(t, r.Kill(t.Context(), id))

		instance, err := e.Inspect(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceStopped, instance.State)

		require.NoError(t, r.Delete(t.Context(), id))
		require.NoError(t, r.Delete(t.Context(), id))

		_, err = r.Inspect(t.Context(), id)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("an image is the engine's to pull", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, runtimeOn(memory.New()).EnsureImage(t.Context(), "anything:at-all"))
	})

	t.Run("what a run uses is what its VM uses", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)
		require.NoError(t, e.SetStats(id, vm.Stats{CPUPercent: 40, MemoryUsed: 50 << 20, NetworkRx: 10, NetworkTx: 20}))

		stats, err := r.Stats(t.Context(), id)
		require.NoError(t, err)

		assert.Equal(t, task.Stats{
			CPUPercent:    40,
			MemoryUsage:   50 << 20,
			MemoryLimit:   200 << 20,
			MemoryPercent: 25,
			NetworkInput:  10,
			NetworkOutput: 20,
		}, stats)
	})
}

func TestRuntime_Logs(t *testing.T) {
	t.Parallel()

	t.Run("a run's log is what its command wrote, and nothing the VM said booting", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		require.NoError(t, e.Log(id, vm.LogSourceKernel, "Linux version 6.12"))
		require.NoError(t, e.Log(id, vm.LogSourceMain, "hello"))
		require.NoError(t, e.Log(id, vm.LogSourceMain, "world"))

		var written bytes.Buffer
		require.NoError(t, r.Logs(t.Context(), id, &written))

		assert.Equal(t, "hello\nworld\n", written.String())
	})

	t.Run("a running run's log is followed as it is written, to its end", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		require.NoError(t, e.Log(id, vm.LogSourceMain, "one"))

		var (
			lock     sync.Mutex
			followed []string
		)

		done := make(chan error, 1)
		go func() {
			done <- r.StreamLogs(t.Context(), id, time.Time{}, func(line task.LogLine) error {
				lock.Lock()
				defer lock.Unlock()

				assert.Equal(t, task.StreamStdout, line.Stream)
				followed = append(followed, line.Content)

				return nil
			})
		}()

		read := func() []string {
			lock.Lock()
			defer lock.Unlock()

			return append([]string(nil), followed...)
		}

		require.Eventually(t, func() bool { return len(read()) == 1 }, 5*time.Second, time.Millisecond)

		// written at the very moment the last line was, as a fast writer's
		// lines are: handed on once all the same.
		require.NoError(t, e.Log(id, vm.LogSourceMain, "two"))
		require.NoError(t, e.Log(id, vm.LogSourceKernel, "not the task's"))
		require.NoError(t, e.Log(id, vm.LogSourceMain, "three"))
		require.NoError(t, e.Exit(id, 0))

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("following did not end with the run")
		}

		assert.Equal(t, []string{"one", "two", "three"}, read())
	})

	t.Run("following stops when the follower does", func(t *testing.T) {
		t.Parallel()

		e := memory.New()
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		assert.NoError(t, r.StreamLogs(ctx, id, time.Time{}, func(task.LogLine) error { return nil }))
	})
}

func TestRuntime_Exec(t *testing.T) {
	t.Parallel()

	shell := func(ctx context.Context, _ string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		_, _ = io.WriteString(stderr, "$ ")
		read, _ := io.ReadAll(stdin)
		_, _ = io.WriteString(stdout, strings.ToUpper(string(read)))

		return 0
	}

	for _, tty := range []bool{true, false} {
		t.Run("a terminal reads what the command says, whichever stream it says it on", func(t *testing.T) {
			t.Parallel()

			e := memory.New(memory.WithExec(shell))
			r := runtimeOn(e)

			id, err := r.Create(t.Context(), snippet(nil))
			require.NoError(t, err)

			session, err := r.Exec(t.Context(), id, task.ExecOptions{Command: []string{"/bin/sh"}, TTY: tty})
			require.NoError(t, err)
			defer session.Close()

			// a terminal reads and writes at once, as the prompt comes before
			// what is typed.
			output := make(chan []byte, 1)
			go func() {
				said, _ := io.ReadAll(session)
				output <- said
			}()

			_, err = io.WriteString(session, "echo hi")
			require.NoError(t, err)
			require.NoError(t, session.(*execSession).session.Stdin().Close())

			var said []byte
			select {
			case said = <-output:
			case <-time.After(5 * time.Second):
				t.Fatal("the command said nothing")
			}

			assert.Contains(t, string(said), "$ ")
			assert.Contains(t, string(said), "ECHO HI")

			require.NoError(t, session.End(t.Context()))
		})
	}

	t.Run("nothing is exec'd into a run that is not running", func(t *testing.T) {
		t.Parallel()

		e := memory.New(memory.WithExec(shell))
		r := runtimeOn(e)

		id, err := r.Create(t.Context(), snippet(nil))
		require.NoError(t, err)
		require.NoError(t, e.Exit(id, 0))

		_, err = r.Exec(t.Context(), id, task.ExecOptions{Command: []string{"/bin/sh"}, TTY: true})
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})
}
