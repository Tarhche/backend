package coderunner

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	deletetask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	tasksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/tasks"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
)

var made = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// run is a snippet the code runner is running, as its task is kept: a job of
// the guest's that came up a second after it was made, with a minute to run.
func run(uuid string) task.Task {
	return task.Task{
		UUID:            uuid,
		Name:            "request-" + uuid,
		Slug:            "request-" + uuid + "-abcde",
		Kind:            task.KindJob,
		OwnerUUID:       task.GuestOwnerUUID,
		Image:           "ghcr.io/tarhche/code-runner:go-1.24-latest",
		ResourceLimits:  task.ResourceLimits{Cpu: 1.5, Memory: 512 << 20, Disk: 512 << 20},
		ExposedPorts:    []port.Port{8080, 3000},
		NetworkPolicy:   network.PolicyIsolated,
		TTL:             time.Minute,
		CurrentState:    task.Running,
		ExpectedState:   task.Running,
		NodeName:        "workload-orchestrator-01",
		LastHeartbeatAt: made.Add(5 * time.Second),
		Deadline:        made.Add(time.Second + time.Minute),
		CreatedAt:       made,
		StartedAt:       made.Add(time.Second),
	}
}

func runsOf(tasks ...task.Task) (*Runs, *tasksMemory.Repository, *logsMock.InMemoryLogRepository, *messagingMock.Recorder) {
	repository := tasksMemory.NewRepository(tasks...)
	logs := logsMock.NewInMemoryRepository()
	producer := &messagingMock.Recorder{}

	return New(repository, producer, deletetask.NewUseCase(repository, logs, producer, translator.Codes{})), repository, logs, producer
}

func TestVM(t *testing.T) {
	t.Parallel()

	t.Run("a run is the guest's machine, given what its task was limited to", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		started := made.Add(time.Second)

		assert.Equal(t, vm.VM{
			UUID:      "run-uuid",
			Name:      "request-run-uuid",
			Slug:      "request-run-uuid-abcde",
			OwnerUUID: task.GuestOwnerUUID,
			Kind:      vm.KindMachine,
			Image:     "ghcr.io/tarhche/code-runner:go-1.24-latest",

			// whole vCPUs, rounded up, and bytes as they were asked for.
			Resources: vm.Resources{CPUs: 2, Memory: 512 << 20, Disk: 512 << 20},
			Ports:     []port.Port{3000, 8080},

			// isolated: its ports are served, and it calls nothing.
			Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			PersistentDisk: false,

			// a minute from when it came up, which is its deadline.
			Lifetime:  time.Minute,
			ExpiresAt: made.Add(time.Second + time.Minute),

			CurrentState:    vm.Running,
			ExpectedState:   vm.Running,
			NodeName:        "workload-orchestrator-01",
			LastHeartbeatAt: made.Add(5 * time.Second),
			CreatedAt:       made,
			StartedAt:       started,
			UpdatedAt:       started,
			ManagedBy:       vm.ManagedByCodeRunner,
		}, VM(&r))
	})

	t.Run("it started when its task says, rather than when its deadline would have it", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.StartedAt = made.Add(2 * time.Second)
		r.FinishedAt = made.Add(9 * time.Second)

		shown := VM(&r)
		assert.Equal(t, made.Add(2*time.Second), shown.StartedAt)
		assert.Equal(t, made.Add(time.Second+time.Minute), shown.ExpiresAt)
		assert.Equal(t, made.Add(9*time.Second), shown.UpdatedAt)
	})

	t.Run("one whose node has not said when it started has no start made up for it", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.StartedAt = time.Time{}

		shown := VM(&r)
		assert.True(t, shown.StartedAt.IsZero())
		assert.Equal(t, made.Add(time.Second+time.Minute), shown.ExpiresAt)
	})

	t.Run("one with no deadline expires its ttl after it started", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.Deadline = time.Time{}

		assert.Equal(t, made.Add(time.Second+time.Minute), VM(&r).ExpiresAt)
	})

	t.Run("one that has not come up has no start, and no expiry yet", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.CurrentState = task.Scheduled
		r.Deadline = time.Time{}
		r.StartedAt = time.Time{}

		shown := VM(&r)
		assert.Equal(t, vm.Scheduled, shown.CurrentState)
		assert.True(t, shown.StartedAt.IsZero())
		assert.True(t, shown.ExpiresAt.IsZero())
		assert.Equal(t, time.Minute, shown.Lifetime)
		assert.Equal(t, made, shown.UpdatedAt)
	})

	t.Run("one that serves nothing has no ports, rather than none to say", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.ExposedPorts = nil
		r.NetworkPolicy = network.PolicyNone

		shown := VM(&r)
		assert.Equal(t, []port.Port{}, shown.Ports)
		assert.Equal(t, vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny}, shown.Network)
	})

	t.Run("its state is its task's, in a VM's words", func(t *testing.T) {
		t.Parallel()

		for state, want := range map[task.State]vm.State{
			task.Created:    vm.Created,
			task.Scheduled:  vm.Scheduled,
			task.Running:    vm.Running,
			task.Stopping:   vm.Stopping,
			task.Stopped:    vm.Stopped,
			task.Completed:  vm.Stopped,
			task.Failed:     vm.Failed,
			task.Restarting: vm.Restarting,
		} {
			r := run("run-uuid")
			r.CurrentState = state
			r.ExpectedState = state
			r.Reason = "the task failed"

			shown := VM(&r)
			assert.Equal(t, want, shown.CurrentState, "%s", state)
			assert.Equal(t, want, shown.ExpectedState, "%s", state)
			assert.Equal(t, "the task failed", shown.Reason)
		}
	})
}

func TestMerge(t *testing.T) {
	t.Parallel()

	at := func(uuid string, minutes int) vm.VM {
		return vm.VM{UUID: uuid, CreatedAt: made.Add(time.Duration(minutes) * time.Minute)}
	}

	uuids := func(vms []vm.VM) []string {
		listed := make([]string, len(vms))
		for i := range vms {
			listed[i] = vms[i].UUID
		}

		return listed
	}

	for name, tt := range map[string]struct {
		vms  []vm.VM
		runs []vm.VM
		want []string
	}{
		"no runs is the VMs as they are": {
			vms:  []vm.VM{at("b", 2), at("a", 1)},
			want: []string{"b", "a"},
		},
		"no VMs is the runs as they are": {
			runs: []vm.VM{at("r2", 2), at("r1", 1)},
			want: []string{"r2", "r1"},
		},
		"runs go where they were made, newest first": {
			vms:  []vm.VM{at("vm-4", 4), at("vm-2", 2), at("vm-1", 1)},
			runs: []vm.VM{at("run-5", 5), at("run-3", 3), at("run-0", 0)},
			want: []string{"run-5", "vm-4", "run-3", "vm-2", "vm-1", "run-0"},
		},
		"of two made at once, the uuid that sorts last goes first": {
			vms:  []vm.VM{at("b", 1)},
			runs: []vm.VM{at("c", 1), at("a", 1)},
			want: []string{"c", "b", "a"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, uuids(Merge(tt.vms, tt.runs)))
		})
	}
}

func TestRuns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("every run is read, a batch at a time, newest first", func(t *testing.T) {
		t.Parallel()

		var tasks []task.Task
		for i := range int(batch) + 5 {
			r := run(fmt.Sprintf("run-%03d", i))
			r.CreatedAt = made.Add(time.Duration(i) * time.Second)
			tasks = append(tasks, r)
		}

		// somebody else's task is no run of the code runner's.
		other := run("task-of-somebody")
		other.OwnerUUID = "owner"

		runs, _, _, _ := runsOf(append(tasks, other)...)

		all, err := runs.All(ctx)
		require.NoError(t, err)
		require.Len(t, all, int(batch)+5)
		assert.Equal(t, fmt.Sprintf("run-%03d", batch+4), all[0].UUID)
		assert.Equal(t, "run-000", all[len(all)-1].UUID)

		for _, shown := range all {
			assert.Equal(t, vm.ManagedByCodeRunner, shown.ManagedBy)
		}
	})

	t.Run("a run is anybody's to read, and nobody's own", func(t *testing.T) {
		t.Parallel()

		other := run("task-of-somebody")
		other.OwnerUUID = "owner"

		runs, _, _, _ := runsOf(run("run"), other)

		found, err := runs.One(ctx, "", "run")
		require.NoError(t, err)
		assert.Equal(t, "run", found.UUID)

		for name, asked := range map[string][2]string{
			"as the guest's own":                  {task.GuestOwnerUUID, "run"},
			"as somebody's own":                   {"owner", "run"},
			"one that is not there":               {"", "missing"},
			"a task that is not the guest's":      {"", "task-of-somebody"},
			"the task of somebody, as theirs":     {"owner", "task-of-somebody"},
			"nothing, which names nothing at all": {"", ""},
		} {
			_, err := runs.One(ctx, asked[0], asked[1])
			assert.ErrorIs(t, err, domain.ErrNotExists, name)
		}
	})

	t.Run("a run is refused what only a VM can be asked, and anything else is as it was", func(t *testing.T) {
		t.Parallel()

		runs, _, _, _ := runsOf(run("run"))
		failed := errors.New("the store is down")

		refused, err := runs.Refused(ctx, "", "run", domain.ErrNotExists)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": CodeRefused}, refused)

		refused, err = runs.Refused(ctx, "owner", "run", domain.ErrNotExists)
		assert.ErrorIs(t, err, domain.ErrNotExists, "nobody's own")
		assert.Nil(t, refused)

		refused, err = runs.Refused(ctx, "", "missing", domain.ErrNotExists)
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, refused)

		refused, err = runs.Refused(ctx, "", "run", failed)
		assert.ErrorIs(t, err, failed, "a failure is not a refusal")
		assert.Nil(t, refused)
	})

	t.Run("deleting one that is gone already is what was asked for", func(t *testing.T) {
		t.Parallel()

		runs, repository, _, producer := runsOf(run("run"))

		gone := run("run")
		require.NoError(t, repository.Delete(ctx, "run"))

		require.NoError(t, runs.Delete(ctx, &gone))
		assert.Empty(t, producer.Messages(), "a task taken away has had its node asked already")
	})
}

func TestLogs(t *testing.T) {
	t.Parallel()

	written := func(lines int) []byte {
		var output []byte
		for i := range lines {
			output = fmt.Appendf(output, "line %d\n", i)
		}

		return output
	}

	t.Run("its output is its log, a line at a time, each a moment after the last", func(t *testing.T) {
		t.Parallel()

		r := run("run")
		r.ExecutionLogs = []byte("hello\r\nhello\n\nbye")

		lines, truncated := Logs(&r, vm.LogOptions{})
		assert.False(t, truncated)
		assert.Equal(t, []noderequest.VMLogLine{
			{At: made, Source: vm.LogSourceMain, Line: "hello"},
			{At: made.Add(1), Source: vm.LogSourceMain, Line: "hello"},
			{At: made.Add(2), Source: vm.LogSourceMain, Line: ""},
			{At: made.Add(3), Source: vm.LogSourceMain, Line: "bye"},
		}, lines)
	})

	t.Run("nothing written is no lines", func(t *testing.T) {
		t.Parallel()

		r := run("run")

		lines, truncated := Logs(&r, vm.LogOptions{})
		assert.Equal(t, []noderequest.VMLogLine{}, lines)
		assert.False(t, truncated)
	})

	for name, tt := range map[string]struct {
		written   int
		options   vm.LogOptions
		first     string
		count     int
		truncated bool
	}{
		"since a line is that line and those after it": {
			written: 10,
			options: vm.LogOptions{Since: made.Add(7)},
			first:   "line 7",
			count:   3,
		},
		"the tail asked for is the last lines": {
			written: 10,
			options: vm.LogOptions{Tail: 2},
			first:   "line 8",
			count:   2,
		},
		"no more than a reply carries, which says it was cut": {
			written:   noderequest.MaxLogLines + 5,
			options:   vm.LogOptions{},
			first:     "line 5",
			count:     noderequest.MaxLogLines,
			truncated: true,
		},
		"nor past it, however many are asked for": {
			written:   noderequest.MaxLogLines + 5,
			options:   vm.LogOptions{Tail: noderequest.MaxLogLines + 100},
			first:     "line 5",
			count:     noderequest.MaxLogLines,
			truncated: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := run("run")
			r.ExecutionLogs = written(tt.written)

			lines, truncated := Logs(&r, tt.options)
			require.Len(t, lines, tt.count)
			assert.Equal(t, tt.first, lines[0].Line)
			assert.Equal(t, tt.truncated, truncated)
		})
	}
}
