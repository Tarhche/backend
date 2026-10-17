package runs_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/khanzadimahdi/testproject/domain/workload/node"
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
		assert.Equal(t, vm.Stats{}, shown.Stats, "with nothing to show its node has not sampled")
	})

	t.Run("what it uses is what its node last sampled of its run, as a vm's is, the blog's way too", func(t *testing.T) {
		t.Parallel()

		sampled := vm.Stats{CPUPercent: 12.5, MemoryUsed: 64 << 20, MemoryLimit: 512 << 20, DiskUsed: 10 << 20, DiskTotal: 512 << 20, NetworkRx: 1, NetworkTx: 2, SampledAt: made.Add(4 * time.Second)}

		r := run("run-uuid", func(t *taskKind.Task) { t.Status.Stats = vmKind.StatsOf(sampled) })

		assert.Equal(t, vmKind.StatsOf(sampled), runs.Manifest(r).Status.Stats)
		assert.Equal(t, sampled, vmKind.Entity(runs.Manifest(r), vmtest.Images.Docker).Stats)

		encoded, err := json.Marshal(runs.Manifest(r).Status)
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &fields))
		assert.JSONEq(t, `{"cpu_percent":12.5,"memory_used":67108864,"memory_limit":536870912,"disk_used":10485760,"disk_total":536870912,"network_rx":1,"network_tx":2,"sampled_at":"2026-10-05T12:00:04Z"}`, string(fields["stats"]))
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

	for _, action := range []string{vmKind.ActionStart, vmKind.ActionRestart, vmKind.ActionRestore, vmKind.ActionReconfigure} {
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

func TestRuns_Act_update(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// running is one of the code runner's runs that came up a moment ago,
	// whose node said it gave its run what its task says: isolated, two
	// cores, 200 MiB of memory and 100 MiB of disk, less than a VM is given
	// at least.
	running := func(change ...func(t *taskKind.Task)) taskKind.Task {
		r := vmtest.Run("run")
		r.Spec.Ports = []port.Port{3000}

		config := r.Spec.Config()
		r.Status.Applied = &config

		for _, c := range change {
			c(&r)
		}

		return r
	}

	updated := func(t *testing.T, w *vmtest.Workload, r taskKind.Task, payload string) (vmKind.VM, domain.ValidationErrors) {
		t.Helper()

		after, gone, refused, err := w.Runs.Act(ctx, raw(t, r), vmKind.ActionUpdate, []byte(payload))
		require.NoError(t, err)
		assert.False(t, gone)

		if len(refused) > 0 {
			return vmKind.VM{}, refused
		}

		v, err := kind.Decode[vmKind.Spec, vmKind.Status](after)
		require.NoError(t, err)

		return v, nil
	}

	commands := func(t *testing.T, w *vmtest.Workload) []kind.ActOnResource {
		t.Helper()

		sent, err := messagingMock.Produced[kind.ActOnResource](w.Producer, kind.ActOnResourceName)
		require.NoError(t, err)

		return sent
	}

	t.Run("a run changed is its task changed, whose node is asked at once to give its run what it was changed to", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(running()))

		after, refused := updated(t, w, running(), `{"name": " a run, renamed ", "ports": [8080, 3000], "network": {"egress": "allow"}, "resources": {"cpus": 3, "memory": 314572800, "disk": 209715200}}`)
		require.Empty(t, refused)

		assert.Equal(t, vmKind.Restarting, after.Status.State, "while its node makes its vm afresh")
		assert.Equal(t, "a run, renamed", after.Metadata.Name)
		assert.Equal(t, []port.Port{3000, 8080}, after.Spec.Ports)
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}, after.Spec.Network)
		assert.Equal(t, vmKind.Resources{CPUs: 3, Memory: 300 * vmtest.MiB, Disk: 200 * vmtest.MiB}, after.Spec.Resources)

		stored, record, _ := w.StoredTask("run")
		assert.Equal(t, "a run, renamed", stored.Metadata.Name)
		assert.Equal(t, []port.Port{3000, 8080}, stored.Spec.Ports)
		assert.Equal(t, network.PolicyPublic, stored.Spec.NetworkPolicy, "isolated, allowed to reach out")
		assert.Equal(t, taskKind.Limits{CPU: 3, Memory: 300 * vmtest.MiB, Disk: 200 * vmtest.MiB}, stored.Spec.Limits)
		assert.Equal(t, taskKind.Restarting, stored.Status.State)
		require.NotNil(t, record.Pending)
		assert.Equal(t, taskKind.ActionReconfigure, record.Pending.Action)

		sent := commands(t, w)
		require.Len(t, sent, 1)
		assert.Equal(t, taskKind.Name, sent[0].Kind)
		assert.Equal(t, "run", sent[0].UUID)
		assert.Equal(t, taskKind.ActionReconfigure, sent[0].Action)
		assert.Equal(t, vmtest.Node, sent[0].Node)
	})

	t.Run("a name alone is its record's, and its node is asked nothing", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(running()))

		after, refused := updated(t, w, running(), `{"name": "renamed"}`)
		require.Empty(t, refused)
		assert.Equal(t, vmKind.Running, after.Status.State)
		assert.Equal(t, "renamed", after.Metadata.Name)
		assert.Empty(t, commands(t, w))
	})

	t.Run("its cores stay what they were unless its vcpus change", func(t *testing.T) {
		t.Parallel()

		halves := func(t *taskKind.Task) {
			t.Spec.Limits.CPU = 1.5
			config := t.Spec.Config()
			t.Status.Applied = &config
		}

		w := vmtest.New(vmtest.WithTasks(running(halves)))

		_, refused := updated(t, w, running(halves), `{"resources": {"cpus": 2, "memory": 314572800, "disk": 104857600}}`)
		require.Empty(t, refused)

		stored, _, _ := w.StoredTask("run")
		assert.Equal(t, taskKind.Limits{CPU: 1.5, Memory: 300 * vmtest.MiB, Disk: 100 * vmtest.MiB}, stored.Spec.Limits)
	})

	t.Run("a network either way names nothing stays as it was, and any of the four a vm's can be is one policy's", func(t *testing.T) {
		t.Parallel()

		for payload, want := range map[string]network.Policy{
			`{"network": {"ingress": "deny"}}`:                    network.PolicyNone,
			`{"network": {"egress": "allow"}}`:                    network.PolicyPublic,
			`{"network": {"ingress": "deny", "egress": "allow"}}`: network.PolicyOutbound,
			`{"network": {"ingress": "allow"}}`:                   network.PolicyIsolated,
		} {
			w := vmtest.New(vmtest.WithTasks(running()))

			_, refused := updated(t, w, running(), payload)
			require.Empty(t, refused, payload)

			stored, _, _ := w.StoredTask("run")
			assert.Equal(t, want, stored.Spec.Policy(), payload)
			assert.Equal(t, []port.Port{3000}, stored.Spec.Ports, "its ports are kept, served once it lets anything in: %s", payload)
		}
	})

	t.Run("its lifetime as it was is no change, and any other is refused", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(running()))

		_, refused := updated(t, w, running(), `{"lifetime": 60000000000}`)
		assert.Empty(t, refused, "a minute, its ttl")

		_, refused = updated(t, w, running(), `{"lifetime": 3600000000000}`)
		assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": runs.CodeRefused}, refused)

		stored, _, _ := w.StoredTask("run")
		assert.Equal(t, time.Minute, stored.Spec.TTL)
	})

	for name, tt := range map[string]struct {
		task    taskKind.Task
		nodes   []node.Node
		payload string
		want    domain.ValidationErrors
	}{
		"a payload that is not one": {
			task:    running(),
			payload: `{"ports": "many"}`,
			want:    domain.ValidationErrors{"payload": "invalid_value"},
		},
		"what a vm's update refuses": {
			task:    running(),
			payload: `{"ports": [0], "network": {"egress": "maybe"}}`,
			want:    domain.ValidationErrors{"ports": "invalid_port", "network.egress": "invalid_access"},
		},
		"more vcpus than one vm may be given": {
			task:    running(),
			payload: `{"resources": {"cpus": 5, "memory": 209715200, "disk": 104857600}}`,
			want:    domain.ValidationErrors{"resources.cpus": "too_large"},
		},
		"less memory than a task's vm takes": {
			task:    running(),
			payload: `{"resources": {"cpus": 2, "memory": 1048576, "disk": 104857600}}`,
			want:    domain.ValidationErrors{"resources.memory": "memory_below_minimum"},
		},
		"a disk that shrinks": {
			task:    running(),
			payload: `{"resources": {"cpus": 2, "memory": 209715200, "disk": 52428800}}`,
			want:    domain.ValidationErrors{"resources.disk": "disk_cannot_shrink"},
		},
		"more than its node has room for": {
			task: running(),
			nodes: []node.Node{func() node.Node {
				n := vmtest.Alive(vmtest.Node)
				n.Capacity.Memory = vmtest.GiB
				n.Capacity.Allocated = vm.Resources{CPUs: 2, Memory: 200 * vmtest.MiB, Disk: 100 * vmtest.MiB}

				return n
			}()},
			payload: `{"resources": {"cpus": 2, "memory": 2147483648, "disk": 104857600}}`,
			want:    domain.ValidationErrors{"resources": "no_capacity"},
		},
		"one restarting already": {
			task:    running(func(t *taskKind.Task) { t.Status.State = taskKind.Restarting }),
			payload: `{"ports": [9000]}`,
			want:    domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"one that ended": {
			task:    running(func(t *taskKind.Task) { t.Status.State = taskKind.Completed }),
			payload: `{"name": "renamed"}`,
			want:    domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithTasks(tt.task)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			w := vmtest.New(opts...)

			_, refused := updated(t, w, tt.task, tt.payload)
			assert.Equal(t, tt.want, refused)
			assert.Empty(t, commands(t, w))

			stored, _, _ := w.StoredTask("run")
			assert.Equal(t, tt.task.Spec, stored.Spec, "and nothing changed")
			assert.Equal(t, tt.task.Metadata.Name, stored.Metadata.Name)
		})
	}

	t.Run("what it leaves as it was is not held to the least a vm may be given, which a run is given less than", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(running()))

		_, refused := updated(t, w, running(), `{"name": "bigger", "resources": {"cpus": 2, "memory": 314572800, "disk": 104857600}}`)
		require.Empty(t, refused, "its 100 MiB disk, under a vm's 1 GiB, is as it was")

		w = vmtest.New(vmtest.WithTasks(running()))

		_, refused = updated(t, w, running(), `{"resources": {"cpus": 2, "memory": 314572800, "disk": 209715200}}`)
		require.Empty(t, refused, "nor is one it changes, but to the least a task may be given")
	})
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

		_, refused, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionState, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": runs.CodeRefused}, refused)
	})
}

func TestRuns_Query_stats(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	sampled := vmKind.StatsOf(vm.Stats{CPUPercent: 12.5, MemoryUsed: 64 << 20, MemoryLimit: 512 << 20, DiskTotal: 512 << 20, SampledAt: made})

	t.Run("what a run uses is its task's sample, asked afresh of the node holding it, as a vm's is", func(t *testing.T) {
		t.Parallel()

		r := run("run")
		w := vmtest.New(vmtest.WithTasks(r))

		w.Requester.Answer = func(_ context.Context, nodeName string, request noderequest.Request) (noderequest.Reply, error) {
			answer, err := json.Marshal(sampled)
			if err != nil {
				return noderequest.Reply{}, err
			}

			return noderequest.Reply{OK: true, Result: answer}, nil
		}

		answer, refused, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionStats, nil)
		require.NoError(t, err)
		require.Empty(t, refused)

		var stats vmKind.Stats
		require.NoError(t, json.Unmarshal(answer, &stats))
		assert.Equal(t, *sampled, stats)

		asked := w.Requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, vmtest.Node, asked[0].NodeName, "its task's node")

		query, err := kind.QueryOf(asked[0].Request)
		require.NoError(t, err)
		assert.Equal(t, taskKind.Name, query.Kind, "its task's own query")
		assert.Equal(t, taskKind.ActionStats, query.Action)
		assert.Equal(t, "run", query.UUID)
		assert.Equal(t, taskKind.Name, query.Resource.Kind, "carrying its task as it is kept, which its node reads it from")
		assert.Equal(t, "run", query.Resource.Metadata.UUID)
	})

	t.Run("one that is not running is not asked, as a vm that is not is not", func(t *testing.T) {
		t.Parallel()

		for _, state := range []kind.State{taskKind.Scheduled, taskKind.Restarting, taskKind.Stopping, taskKind.Completed, taskKind.Failed} {
			r := run("run", func(t *taskKind.Task) { t.Status.State = state })
			w := vmtest.New(vmtest.WithTasks(r))

			_, refused, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionStats, nil)
			require.Empty(t, refused)

			var notRunning *noderequest.Error
			require.ErrorAs(t, err, &notRunning, "%s", state)
			assert.Equal(t, noderequest.CodeNotRunning, notRunning.Code, "%s", state)
			assert.Empty(t, w.Requester.Asked(), "%s: its node is asked nothing", state)
		}
	})

	t.Run("nor is one on no node", func(t *testing.T) {
		t.Parallel()

		r := run("run", func(t *taskKind.Task) { t.Metadata.Node = "" })
		w := vmtest.New(vmtest.WithTasks(r))

		_, _, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionStats, nil)

		var notRunning *noderequest.Error
		require.ErrorAs(t, err, &notRunning)
		assert.Equal(t, noderequest.CodeNotRunning, notRunning.Code)
		assert.Empty(t, w.Requester.Asked())
	})

	t.Run("what its node refused is its refusal, and a node that does not answer in time says so", func(t *testing.T) {
		t.Parallel()

		r := run("run")

		for name, tt := range map[string]struct {
			reply noderequest.Reply
			err   error
			code  noderequest.Code
		}{
			"refused":      {reply: noderequest.Failed(fmt.Errorf("%w: its run is down", vm.ErrNotRunning)), code: noderequest.CodeNotRunning},
			"not in time":  {err: context.DeadlineExceeded, code: noderequest.CodeTimeout},
			"not answered": {err: errors.New("no responders"), code: noderequest.CodeInternal},
		} {
			w := vmtest.New(vmtest.WithTasks(r))
			w.Requester.Answer = func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return tt.reply, tt.err
			}

			_, _, err := w.Runs.Query(ctx, raw(t, r), vmKind.ActionStats, nil)

			var refused *noderequest.Error
			require.ErrorAs(t, err, &refused, name)
			assert.Equal(t, tt.code, refused.Code, name)
		}
	})

	t.Run("one that went in the meantime is not there", func(t *testing.T) {
		t.Parallel()

		_, _, err := vmtest.New().Runs.Query(ctx, raw(t, run("run")), vmKind.ActionStats, nil)
		assert.ErrorIs(t, err, domain.ErrNotExists)
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
