package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	deletetask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
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

type fixture struct {
	runs     *Runs
	tasks    *tasksMemory.Repository
	producer *messagingMock.Recorder
}

func runsOf(tasks ...task.Task) fixture {
	repository := tasksMemory.NewRepository(tasks...)
	producer := &messagingMock.Recorder{}

	return fixture{
		runs:     New(repository, producer, deletetask.NewUseCase(repository, logsMock.NewInMemoryRepository(), producer, translator.Codes{})),
		tasks:    repository,
		producer: producer,
	}
}

func TestManifest(t *testing.T) {
	t.Parallel()

	t.Run("a run is the guest's machine, given what its task was limited to, labelled as the code runner's", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		started := made.Add(time.Second)

		assert.Equal(t, vmKind.VM{
			Kind: vmKind.Name,
			Metadata: kind.Metadata{
				UUID:      "run-uuid",
				Name:      "request-run-uuid",
				Slug:      "request-run-uuid-abcde",
				OwnerUUID: task.GuestOwnerUUID,
				Labels: map[string]string{
					vmKind.LabelFlavor:    "machine",
					vmKind.LabelManagedBy: vmKind.ManagedByCodeRunner,
				},
				Node: "workload-orchestrator-01",

				// a minute from when it came up, which is its deadline.
				Lifetime:  time.Minute,
				ExpiresAt: made.Add(time.Second + time.Minute),

				CreatedAt: made,
				UpdatedAt: started,
			},
			Spec: vmKind.Spec{
				Flavor: vmKind.FlavorMachine,
				Image:  "ghcr.io/tarhche/code-runner:go-1.24-latest",

				// whole vCPUs, rounded up, and bytes as they were asked for.
				Resources: vmKind.Resources{CPUs: 2, Memory: 512 << 20, Disk: 512 << 20},
				Ports:     []port.Port{3000, 8080},

				// isolated: its ports are served, and it calls nothing.
				Network: vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			},
			Status: vmKind.Status{
				Status: kind.Status{
					State:      vmKind.Running,
					Expected:   vmKind.Running,
					ObservedAt: made.Add(5 * time.Second),
				},
				StartedAt: started,
			},
		}, Manifest(&r))
	})

	t.Run("it started when its task says, rather than when its deadline would have it", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.StartedAt = made.Add(2 * time.Second)
		r.FinishedAt = made.Add(9 * time.Second)

		shown := Manifest(&r)
		assert.Equal(t, made.Add(2*time.Second), shown.Status.StartedAt)
		assert.Equal(t, made.Add(time.Second+time.Minute), shown.Metadata.ExpiresAt)
		assert.Equal(t, made.Add(9*time.Second), shown.Metadata.UpdatedAt)
	})

	t.Run("one with no deadline expires its ttl after it started", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.Deadline = time.Time{}

		assert.Equal(t, made.Add(time.Second+time.Minute), Manifest(&r).Metadata.ExpiresAt)
	})

	t.Run("one that has not come up has no start, and no expiry yet", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.CurrentState = task.Scheduled
		r.Deadline = time.Time{}
		r.StartedAt = time.Time{}

		shown := Manifest(&r)
		assert.Equal(t, vmKind.Scheduled, shown.Status.State)
		assert.True(t, shown.Status.StartedAt.IsZero())
		assert.True(t, shown.Metadata.ExpiresAt.IsZero())
		assert.Equal(t, time.Minute, shown.Metadata.Lifetime)
		assert.Equal(t, made, shown.Metadata.UpdatedAt)
	})

	t.Run("one that serves nothing has no ports, rather than none to say", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.ExposedPorts = nil
		r.NetworkPolicy = network.PolicyNone

		shown := Manifest(&r)
		assert.Equal(t, []port.Port{}, shown.Spec.Ports)
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny}, shown.Spec.Network)
	})

	t.Run("and one bound to ports serves those, once each", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")
		r.PortBindings = []port.PortMap{{8080: {{HostPort: 1}}}, {9090: {{HostPort: 2}}}}

		assert.Equal(t, []port.Port{3000, 8080, 9090}, Manifest(&r).Spec.Ports)
	})

	t.Run("its state is its task's, in a vm's words", func(t *testing.T) {
		t.Parallel()

		for state, want := range map[task.State]kind.State{
			task.Created:    vmKind.Created,
			task.Scheduled:  vmKind.Scheduled,
			task.Running:    vmKind.Running,
			task.Stopping:   vmKind.Stopping,
			task.Stopped:    vmKind.Stopped,
			task.Completed:  vmKind.Stopped,
			task.Failed:     vmKind.Failed,
			task.Restarting: vmKind.Restarting,
		} {
			r := run("run-uuid")
			r.CurrentState = state
			r.ExpectedState = state
			r.Reason = "the task failed"

			shown := Manifest(&r)
			assert.Equal(t, want, shown.Status.State, "%s", state)
			assert.Equal(t, want, shown.Status.Expected, "%s", state)
			assert.Equal(t, "the task failed", shown.Status.Reason)
		}
	})

	t.Run("and the vm it is shown as is the code runner's, the blog's way", func(t *testing.T) {
		t.Parallel()

		r := run("run-uuid")

		shown := vmKind.Entity(Manifest(&r))
		assert.Equal(t, vm.ManagedByCodeRunner, shown.ManagedBy)
		assert.Equal(t, vm.KindMachine, shown.Kind)
		assert.Equal(t, vm.Running, shown.CurrentState)
		assert.Equal(t, made.Add(5*time.Second), shown.LastHeartbeatAt)
	})
}

func TestRuns_All(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("every run is read, a batch at a time, newest first, and nobody else's task", func(t *testing.T) {
		t.Parallel()

		var tasks []task.Task
		for i := range int(batch) + 5 {
			r := run(fmt.Sprintf("run-%03d", i))
			r.CreatedAt = made.Add(time.Duration(i) * time.Second)
			tasks = append(tasks, r)
		}

		other := run("task-of-somebody")
		other.OwnerUUID = "owner"

		all, err := runsOf(append(tasks, other)...).runs.All(ctx)
		require.NoError(t, err)
		require.Len(t, all, int(batch)+5)
		assert.Equal(t, fmt.Sprintf("run-%03d", batch+4), all[0].Metadata.UUID)
		assert.Equal(t, "run-000", all[len(all)-1].Metadata.UUID)

		for _, shown := range all {
			assert.Equal(t, vmKind.ManagedByCodeRunner, shown.Metadata.Labels[vmKind.LabelManagedBy])
		}
	})

	t.Run("of two made at once, the uuid that sorts last goes first", func(t *testing.T) {
		t.Parallel()

		all, err := runsOf(run("a"), run("c"), run("b")).runs.All(ctx)
		require.NoError(t, err)

		uuids := make([]string, len(all))
		for i := range all {
			uuids[i] = all[i].Metadata.UUID
		}

		assert.Equal(t, []string{"c", "b", "a"}, uuids)
	})
}

func TestRuns_One(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	other := run("task-of-somebody")
	other.OwnerUUID = "owner"

	f := runsOf(run("run"), other)

	found, err := f.runs.One(ctx, "run")
	require.NoError(t, err)
	assert.Equal(t, "run", found.Metadata.UUID)

	for name, uuid := range map[string]string{
		"one that is not there":               "missing",
		"a task that is not the guest's":      "task-of-somebody",
		"nothing, which names nothing at all": "",
	} {
		_, err := f.runs.One(ctx, uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists, name)
	}
}

func TestRuns_Act(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	raw := func(t *testing.T, r task.Task) kind.Raw {
		t.Helper()

		encoded, err := kind.Encode(Manifest(&r))
		require.NoError(t, err)

		return encoded
	}

	t.Run("a run stopped is its task stopped, which its node is asked to do", func(t *testing.T) {
		t.Parallel()

		f := runsOf(run("run"))

		after, gone, refused, err := f.runs.Act(ctx, raw(t, run("run")), vmKind.ActionStop, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.False(t, gone)

		stopping, err := kind.Decode[vmKind.Spec, vmKind.Status](after)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Stopping, stopping.Status.State)
		assert.Equal(t, vmKind.Stopped, stopping.Status.Expected)

		stored, _ := f.tasks.Stored("run")
		assert.Equal(t, task.Stopping, stored.CurrentState)
		assert.Equal(t, task.Stopped, stored.ExpectedState)

		var asked events.TaskStoppageRequested
		require.True(t, f.producer.Last(events.TaskStoppageRequestedName, &asked))
		assert.Equal(t, "run", asked.UUID)
	})

	t.Run("one that cannot get there from where it is says so, and is to stop all the same", func(t *testing.T) {
		t.Parallel()

		stopping := run("run")
		stopping.CurrentState = task.Stopping

		f := runsOf(stopping)

		_, _, refused, err := f.runs.Act(ctx, raw(t, stopping), vmKind.ActionStop, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, refused)
		assert.Empty(t, f.producer.Messages())

		stored, _ := f.tasks.Stored("run")
		assert.Equal(t, task.Stopped, stored.ExpectedState)
	})

	t.Run("a run deleted is its task taken away", func(t *testing.T) {
		t.Parallel()

		f := runsOf(run("run"))

		_, gone, refused, err := f.runs.Act(ctx, raw(t, run("run")), vmKind.ActionDelete, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.True(t, gone)

		_, kept := f.tasks.Stored("run")
		assert.False(t, kept)
	})

	t.Run("one that went in the meantime is gone already", func(t *testing.T) {
		t.Parallel()

		f := runsOf(run("run"))
		require.NoError(t, f.tasks.Delete(ctx, "run"))

		_, _, _, err := f.runs.Act(ctx, raw(t, run("run")), vmKind.ActionDelete, nil)
		assert.ErrorIs(t, err, domain.ErrNotExists, "there is nothing to take away")
	})

	for _, action := range []string{vmKind.ActionStart, vmKind.ActionRestart, vmKind.ActionUpdate, vmKind.ActionRestore, vmKind.ActionReconfigure} {
		t.Run("a run is refused a "+action, func(t *testing.T) {
			t.Parallel()

			f := runsOf(run("run"))

			_, gone, refused, err := f.runs.Act(ctx, raw(t, run("run")), action, nil)
			require.NoError(t, err)
			assert.False(t, gone)
			assert.Equal(t, domain.ValidationErrors{"vm": CodeRefused}, refused)
			assert.Empty(t, f.producer.Messages())

			stored, _ := f.tasks.Stored("run")
			assert.Equal(t, task.Running, stored.CurrentState)
		})
	}
}

func TestRuns_Query(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	r := run("run")
	r.ExecutionLogs = []byte("hello\nbye\n")

	encoded, err := kind.Encode(Manifest(&r))
	require.NoError(t, err)

	f := runsOf(r)

	t.Run("what a run wrote is its log", func(t *testing.T) {
		t.Parallel()

		answer, refused, err := f.runs.Query(ctx, encoded, vmKind.ActionLogs, []byte(`{"tail":1}`))
		require.NoError(t, err)
		require.Empty(t, refused)

		var logs vmKind.Logs
		require.NoError(t, json.Unmarshal(answer, &logs))
		require.Len(t, logs.Lines, 1)
		assert.Equal(t, "bye", logs.Lines[0].Line)
	})

	t.Run("a payload that is not one is refused", func(t *testing.T) {
		t.Parallel()

		_, refused, err := f.runs.Query(ctx, encoded, vmKind.ActionLogs, []byte(`{"tail":"many"}`))
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"payload": "invalid_value"}, refused)
	})

	t.Run("and nothing else is asked of one", func(t *testing.T) {
		t.Parallel()

		_, refused, err := f.runs.Query(ctx, encoded, vmKind.ActionStats, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": CodeRefused}, refused)
	})
}

func TestRuns_Refused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	f := runsOf(run("run"))
	failed := errors.New("the store is down")

	refused, err := f.runs.Refused(ctx, "", "run", domain.ErrNotExists)
	require.NoError(t, err)
	assert.Equal(t, domain.ValidationErrors{"vm": CodeRefused}, refused)

	refused, err = f.runs.Refused(ctx, "owner", "run", domain.ErrNotExists)
	assert.ErrorIs(t, err, domain.ErrNotExists, "nobody's own")
	assert.Nil(t, refused)

	refused, err = f.runs.Refused(ctx, "", "missing", domain.ErrNotExists)
	assert.ErrorIs(t, err, domain.ErrNotExists)
	assert.Nil(t, refused)

	refused, err = f.runs.Refused(ctx, "", "run", failed)
	assert.ErrorIs(t, err, failed, "a failure is not a refusal")
	assert.Nil(t, refused)
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

		assert.Equal(t, vmKind.Logs{Lines: []vmKind.LogLine{
			{At: made, Source: vm.LogSourceMain, Line: "hello"},
			{At: made.Add(1), Source: vm.LogSourceMain, Line: "hello"},
			{At: made.Add(2), Source: vm.LogSourceMain, Line: ""},
			{At: made.Add(3), Source: vm.LogSourceMain, Line: "bye"},
		}}, Logs(&r, vmKind.LogsPayload{}))
	})

	t.Run("nothing written is no lines", func(t *testing.T) {
		t.Parallel()

		r := run("run")

		assert.Equal(t, vmKind.Logs{Lines: []vmKind.LogLine{}}, Logs(&r, vmKind.LogsPayload{}))
	})

	for name, tt := range map[string]struct {
		written   int
		options   vmKind.LogsPayload
		first     string
		count     int
		truncated bool
	}{
		"since a line is that line and those after it": {
			written: 10,
			options: vmKind.LogsPayload{Since: made.Add(7)},
			first:   "line 7",
			count:   3,
		},
		"the tail asked for is the last lines": {
			written: 10,
			options: vmKind.LogsPayload{Tail: 2},
			first:   "line 8",
			count:   2,
		},
		"no more than a reply carries, which says it was cut": {
			written:   noderequest.MaxLogLines + 5,
			first:     "line 5",
			count:     noderequest.MaxLogLines,
			truncated: true,
		},
		"nor past it, however many are asked for": {
			written:   noderequest.MaxLogLines + 5,
			options:   vmKind.LogsPayload{Tail: noderequest.MaxLogLines + 100},
			first:     "line 5",
			count:     noderequest.MaxLogLines,
			truncated: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := run("run")
			r.ExecutionLogs = written(tt.written)

			logs := Logs(&r, tt.options)
			require.Len(t, logs.Lines, tt.count)
			assert.Equal(t, tt.first, logs.Lines[0].Line)
			assert.Equal(t, tt.truncated, logs.Truncated)
		})
	}
}
