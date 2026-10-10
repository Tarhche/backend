package runs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/runs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

var made = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// run is a snippet the code runner is running, as its task is kept: a job of
// the guest's that came up a second after it was made, with a minute to run.
func run(uuid string, change ...func(t *taskKind.Task)) taskKind.Task {
	none := 0

	t := taskKind.Task{
		Kind: taskKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "request-" + uuid,
			Slug:      "request-" + uuid + "-abcde",
			OwnerUUID: task.GuestOwnerUUID,
			Node:      vmtest.Node,
			CreatedAt: made,
			UpdatedAt: made.Add(5 * time.Second),
		},
		Spec: taskKind.Spec{
			Kind:          task.KindJob,
			Image:         "ghcr.io/tarhche/code-runner:go-1.24-latest",
			Limits:        taskKind.Limits{CPU: 1.5, Memory: 512 << 20, Disk: 512 << 20},
			Ports:         []port.Port{8080, 3000},
			NetworkPolicy: network.PolicyIsolated,
			TTL:           time.Minute,
			MaxRetries:    &none,
		},
		Status: taskKind.Status{
			Status: kind.Status{State: taskKind.Running, Expected: taskKind.Running, Since: made.Add(time.Second), ObservedAt: made.Add(5 * time.Second)},
			Run: &taskKind.Run{
				ID:        "execution-" + uuid,
				Name:      "request-" + uuid,
				Slug:      "request-" + uuid + "-abcde",
				Kind:      task.KindJob,
				StartedAt: made.Add(time.Second),
				Deadline:  made.Add(time.Second + time.Minute),
			},
		},
	}

	for _, c := range change {
		c(&t)
	}

	return t
}

func raw(t *testing.T, r taskKind.Task) kind.Raw {
	t.Helper()

	encoded, err := kind.Encode(runs.Manifest(r))
	require.NoError(t, err)

	return encoded
}

func TestManifest(t *testing.T) {
	t.Parallel()

	t.Run("a run is the guest's machine, given what its task was limited to, labelled as the code runner's", func(t *testing.T) {
		t.Parallel()

		started := made.Add(time.Second)

		assert.Equal(t, vmKind.VM{
			Kind: vmKind.Name,
			Metadata: kind.Metadata{
				UUID:      "run-uuid",
				Name:      "request-run-uuid",
				Slug:      "request-run-uuid-abcde",
				OwnerUUID: task.GuestOwnerUUID,
				Labels: map[string]string{
					vmKind.LabelManagedBy: vmKind.ManagedByCodeRunner,
				},
				Node: vmtest.Node,

				// a minute from when it came up, which is its deadline.
				Lifetime:  time.Minute,
				ExpiresAt: made.Add(time.Second + time.Minute),

				CreatedAt: made,
				UpdatedAt: started,
			},
			Spec: vmKind.Spec{
				Image: "ghcr.io/tarhche/code-runner:go-1.24-latest",

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
		}, runs.Manifest(run("run-uuid")))
	})

	t.Run("it started when its run says, and ended when it came to rest", func(t *testing.T) {
		t.Parallel()

		shown := runs.Manifest(run("run-uuid", func(t *taskKind.Task) {
			t.Status.Run.StartedAt = made.Add(2 * time.Second)
			t.Status.State = taskKind.Completed
			t.Status.Since = made.Add(9 * time.Second)
		}))

		assert.Equal(t, made.Add(2*time.Second), shown.Status.StartedAt)
		assert.Equal(t, made.Add(time.Second+time.Minute), shown.Metadata.ExpiresAt, "its deadline, as its node set it")
		assert.Equal(t, made.Add(9*time.Second), shown.Metadata.UpdatedAt)
	})

	t.Run("one with no deadline expires its ttl after it started", func(t *testing.T) {
		t.Parallel()

		shown := runs.Manifest(run("run-uuid", func(t *taskKind.Task) { t.Status.Run.Deadline = time.Time{} }))

		assert.Equal(t, made.Add(time.Second+time.Minute), shown.Metadata.ExpiresAt)
	})

	t.Run("one that has not come up has no start, and no expiry yet", func(t *testing.T) {
		t.Parallel()

		shown := runs.Manifest(run("run-uuid", func(t *taskKind.Task) {
			t.Status.State = taskKind.Scheduled
			t.Status.Run = nil
		}))

		assert.Equal(t, vmKind.Scheduled, shown.Status.State)
		assert.True(t, shown.Status.StartedAt.IsZero())
		assert.True(t, shown.Metadata.ExpiresAt.IsZero())
		assert.Equal(t, time.Minute, shown.Metadata.Lifetime)
		assert.Equal(t, made, shown.Metadata.UpdatedAt)
	})

	t.Run("one that serves nothing has no ports, rather than none to say", func(t *testing.T) {
		t.Parallel()

		shown := runs.Manifest(run("run-uuid", func(t *taskKind.Task) {
			t.Spec.Ports = nil
			t.Spec.NetworkPolicy = network.PolicyNone
		}))

		assert.Equal(t, []port.Port{}, shown.Spec.Ports)
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny}, shown.Spec.Network)
	})

	t.Run("its state is its task's, in a vm's words", func(t *testing.T) {
		t.Parallel()

		for state, want := range map[kind.State]kind.State{
			taskKind.Created:    vmKind.Created,
			taskKind.Scheduled:  vmKind.Scheduled,
			taskKind.Running:    vmKind.Running,
			taskKind.Stopping:   vmKind.Stopping,
			taskKind.Stopped:    vmKind.Stopped,
			taskKind.Completed:  vmKind.Stopped,
			taskKind.Failed:     vmKind.Failed,
			taskKind.Restarting: vmKind.Restarting,
			taskKind.Deleting:   vmKind.Deleting,
		} {
			shown := runs.Manifest(run("run-uuid", func(t *taskKind.Task) {
				t.Status.State = state
				t.Status.Expected = state
				t.Status.Reason = "the task failed"
			}))

			assert.Equal(t, want, shown.Status.State, "%s", state)
			assert.Equal(t, want, shown.Status.Expected, "%s", state)
			assert.Equal(t, "the task failed", shown.Status.Reason)
		}
	})

	t.Run("and the vm it is shown as is the code runner's, the blog's way", func(t *testing.T) {
		t.Parallel()

		shown := vmKind.Entity(runs.Manifest(run("run-uuid")), vmtest.Images.Docker)
		assert.Equal(t, vm.ManagedByCodeRunner, shown.ManagedBy)
		assert.Equal(t, vm.KindMachine, shown.Kind)
		assert.Equal(t, vm.Running, shown.CurrentState)
		assert.Equal(t, made.Add(5*time.Second), shown.LastHeartbeatAt)
	})
}

func TestRuns_All(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("every run, newest first, and nobody else's task, nor any vm", func(t *testing.T) {
		t.Parallel()

		var tasks []taskKind.Task
		for i := range 105 {
			tasks = append(tasks, run(fmt.Sprintf("run-%03d", i), func(t *taskKind.Task) {
				t.Metadata.CreatedAt = made.Add(time.Duration(i) * time.Second)
			}))
		}

		tasks = append(tasks, run("task-of-somebody", func(t *taskKind.Task) { t.Metadata.OwnerUUID = "owner" }))

		w := vmtest.New(vmtest.WithTasks(tasks...), vmtest.WithVMs(vmtest.Running("01", "owner")))

		all, err := w.Runs.All(ctx)
		require.NoError(t, err)
		require.Len(t, all, 105)
		assert.Equal(t, "run-104", all[0].Metadata.UUID)
		assert.Equal(t, "run-000", all[len(all)-1].Metadata.UUID)

		for _, shown := range all {
			assert.Equal(t, vmKind.Name, shown.Kind)
			assert.Equal(t, vmKind.ManagedByCodeRunner, shown.Metadata.Labels[vmKind.LabelManagedBy])
		}
	})

	t.Run("of two made at once, the uuid that sorts last goes first", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(run("a"), run("c"), run("b")))

		all, err := w.Runs.All(ctx)
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

	w := vmtest.New(vmtest.WithTasks(run("run"), run("task-of-somebody", func(t *taskKind.Task) { t.Metadata.OwnerUUID = "owner" })))

	found, err := w.Runs.One(ctx, "run")
	require.NoError(t, err)
	assert.Equal(t, "run", found.Metadata.UUID)
	assert.Equal(t, vmKind.Name, found.Kind)

	for name, uuid := range map[string]string{
		"one that is not there":               "missing",
		"a task that is not the guest's":      "task-of-somebody",
		"nothing, which names nothing at all": "",
	} {
		_, err := w.Runs.One(ctx, uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists, name)
	}
}

func TestRuns_Act(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	commands := func(t *testing.T, w *vmtest.Workload) []kind.ActOnResource {
		t.Helper()

		sent, err := messagingMock.Produced[kind.ActOnResource](w.Producer, kind.ActOnResourceName)
		require.NoError(t, err)

		return sent
	}

	t.Run("a run stopped is its task stopped, which its node is asked to do", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(run("run")))

		after, gone, refused, err := w.Runs.Act(ctx, raw(t, run("run")), vmKind.ActionStop, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.False(t, gone)

		stopping, err := kind.Decode[vmKind.Spec, vmKind.Status](after)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Stopping, stopping.Status.State)
		assert.Equal(t, vmKind.Stopped, stopping.Status.Expected)

		stored, record, _ := w.StoredTask("run")
		assert.Equal(t, taskKind.Stopping, stored.Status.State)
		assert.Equal(t, taskKind.Stopped, stored.Status.Expected)
		require.NotNil(t, record.Pending)
		assert.Equal(t, taskKind.ActionStop, record.Pending.Action)

		sent := commands(t, w)
		require.Len(t, sent, 1)
		assert.Equal(t, taskKind.Name, sent[0].Kind)
		assert.Equal(t, "run", sent[0].UUID)
		assert.Equal(t, taskKind.ActionStop, sent[0].Action)
		assert.Equal(t, vmtest.Node, sent[0].Node)
	})

	t.Run("one that cannot get there from where it is says so, and is to stop all the same", func(t *testing.T) {
		t.Parallel()

		stopping := run("run", func(t *taskKind.Task) { t.Status.State = taskKind.Stopping })
		w := vmtest.New(vmtest.WithTasks(stopping))

		_, _, refused, err := w.Runs.Act(ctx, raw(t, stopping), vmKind.ActionStop, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, refused)
		assert.Empty(t, w.Producer.Messages())

		stored, _, _ := w.StoredTask("run")
		assert.Equal(t, taskKind.Stopped, stored.Status.Expected)
	})

	t.Run("one being deleted is refused a stop, and is still to be deleted", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(run("run")))

		_, _, refused, err := w.Runs.Act(ctx, raw(t, run("run")), vmKind.ActionDelete, nil)
		require.NoError(t, err)
		require.Empty(t, refused)

		_, _, refused, err = w.Runs.Act(ctx, raw(t, run("run")), vmKind.ActionStop, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, refused)

		stored, _, _ := w.StoredTask("run")
		assert.Equal(t, taskKind.Deleting, stored.Status.State)
		assert.Equal(t, kind.Deleted, stored.Status.Expected)
	})

	t.Run("a run deleted is its task asked to be taken away, by its node", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(run("run")))

		after, gone, refused, err := w.Runs.Act(ctx, raw(t, run("run")), vmKind.ActionDelete, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.False(t, gone, "it goes once its node has taken it away")

		deleting, err := kind.Decode[vmKind.Spec, vmKind.Status](after)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Deleting, deleting.Status.State)

		sent := commands(t, w)
		require.Len(t, sent, 1)
		assert.Equal(t, taskKind.ActionDelete, sent[0].Action)

		_, _, refused, err = w.Runs.Act(ctx, raw(t, run("run")), vmKind.ActionDelete, nil)
		require.NoError(t, err)
		assert.Empty(t, refused)
		assert.Len(t, commands(t, w), 1, "one being deleted is left to it")
	})

	t.Run("one on no node is gone at once, there being nothing anywhere to take away", func(t *testing.T) {
		t.Parallel()

		unplaced := run("run", func(t *taskKind.Task) {
			t.Metadata.Node = ""
			t.Status.State = taskKind.Created
			t.Status.Run = nil
		})
		w := vmtest.New(vmtest.WithTasks(unplaced))

		_, gone, refused, err := w.Runs.Act(ctx, raw(t, unplaced), vmKind.ActionDelete, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.True(t, gone)

		_, _, kept := w.StoredTask("run")
		assert.False(t, kept)
	})

	t.Run("one that went in the meantime is gone already", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		_, _, _, err := w.Runs.Act(ctx, raw(t, run("run")), vmKind.ActionDelete, nil)
		assert.ErrorIs(t, err, domain.ErrNotExists, "there is nothing to take away")
	})

	for _, action := range []string{vmKind.ActionStart, vmKind.ActionRestart, vmKind.ActionUpdate, vmKind.ActionRestore, vmKind.ActionReconfigure} {
		t.Run("a run is refused a "+action, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithTasks(run("run")))

			_, gone, refused, err := w.Runs.Act(ctx, raw(t, run("run")), action, nil)
			require.NoError(t, err)
			assert.False(t, gone)
			assert.Equal(t, domain.ValidationErrors{"vm": runs.CodeRefused}, refused)
			assert.Empty(t, w.Producer.Messages())

			stored, _, _ := w.StoredTask("run")
			assert.Equal(t, taskKind.Running, stored.Status.State)
		})
	}
}

func TestRuns_Query(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	r := run("run", func(t *taskKind.Task) { t.Status.Run.Output = "hello\nbye\n" })
	w := vmtest.New(vmtest.WithTasks(r))

	t.Run("what a run wrote is its log", func(t *testing.T) {
		t.Parallel()

		answer, refused, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionLogs, []byte(`{"tail":1}`))
		require.NoError(t, err)
		require.Empty(t, refused)

		var logs vmKind.Logs
		require.NoError(t, json.Unmarshal(answer, &logs))
		require.Len(t, logs.Lines, 1)
		assert.Equal(t, "bye", logs.Lines[0].Line)
	})

	t.Run("a payload that is not one is refused", func(t *testing.T) {
		t.Parallel()

		_, refused, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionLogs, []byte(`{"tail":"many"}`))
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"payload": "invalid_value"}, refused)
	})

	t.Run("and nothing else is asked of one", func(t *testing.T) {
		t.Parallel()

		_, refused, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionStats, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": runs.CodeRefused}, refused)
	})
}

func TestLogs(t *testing.T) {
	t.Parallel()

	written := func(lines int) string {
		var output string
		for i := range lines {
			output += fmt.Sprintf("line %d\n", i)
		}

		return output
	}

	t.Run("its output is its log, a line at a time, each a moment after the last", func(t *testing.T) {
		t.Parallel()

		r := run("run", func(t *taskKind.Task) { t.Status.Run.Output = "hello\r\nhello\n\nbye" })

		assert.Equal(t, vmKind.Logs{Lines: []vmKind.LogLine{
			{At: made, Source: vm.LogSourceMain, Line: "hello"},
			{At: made.Add(1), Source: vm.LogSourceMain, Line: "hello"},
			{At: made.Add(2), Source: vm.LogSourceMain, Line: ""},
			{At: made.Add(3), Source: vm.LogSourceMain, Line: "bye"},
		}}, runs.Logs(r, vmKind.LogsPayload{}))
	})

	t.Run("nothing written is no lines", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, vmKind.Logs{Lines: []vmKind.LogLine{}}, runs.Logs(run("run"), vmKind.LogsPayload{}))
		assert.Equal(t, vmKind.Logs{Lines: []vmKind.LogLine{}}, runs.Logs(run("run", func(t *taskKind.Task) { t.Status.Run = nil }), vmKind.LogsPayload{}))
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

			r := run("run", func(t *taskKind.Task) { t.Status.Run.Output = written(tt.written) })

			logs := runs.Logs(r, tt.options)
			require.Len(t, logs.Lines, tt.count)
			assert.Equal(t, tt.first, logs.Lines[0].Line)
			assert.Equal(t, tt.truncated, logs.Truncated)
		})
	}
}
