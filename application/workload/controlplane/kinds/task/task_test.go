package task

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
)

var now = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// fixture is the strategy over nodes, the logs it keeps, and slugs that
// are taken.
type fixture struct {
	tasks *Tasks
	logs  *logsMock.InMemoryLogRepository
}

func strategy(nodes []node.Node, taken ...string) fixture {
	logs := logsMock.NewInMemoryRepository()

	return fixture{
		tasks: New(Dependencies{
			Nodes:     nodesMemory.NewRepository(nodes...),
			Scheduler: roundrobin.New(),
			Slugs: []slugs.Taken{func(_ context.Context, slug string) (bool, error) {
				for _, held := range taken {
					if held == slug {
						return true, nil
					}
				}

				return false, nil
			}},
			Logs: logs,
			Now:  func() time.Time { return now },
		}),
		logs: logs,
	}
}

// spoke is a node that last spoke ago.
func spoke(name string, ago time.Duration) node.Node {
	return node.Node{Name: name, LastHeartbeatAt: now.Add(-ago)}
}

// snippet is what the code runner asks for: a job of the guest's.
func snippet() taskKind.Task {
	none := 0

	return taskKind.Task{
		Kind:     taskKind.Name,
		Metadata: kind.Metadata{Name: "0199b3c2-request", OwnerUUID: task.GuestOwnerUUID},
		Spec: taskKind.Spec{
			Kind:       task.KindJob,
			Image:      "ghcr.io/tarhche/code-runner:go-1.24-latest",
			Command:    []string{"--timeout", "30", "package main"},
			Ports:      []port.Port{8080, 3000, 8080},
			TTL:        time.Minute,
			Limits:     taskKind.Limits{CPU: 2, Memory: 512 << 20, Disk: 512 << 20},
			MaxRetries: &none,
		},
	}
}

func TestTasks_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a snippet is admitted as asked, placed on a node that spoke lately", func(t *testing.T) {
		t.Parallel()

		admitted, invalid, err := strategy([]node.Node{spoke("node-01", time.Second)}).tasks.Admit(ctx, snippet())
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, taskKind.Name, admitted.Kind)
		assert.Equal(t, "0199b3c2-request", admitted.Metadata.Name)
		assert.Regexp(t, `^0199b3c2-request-[a-z]{5}$`, admitted.Metadata.Slug)
		assert.Equal(t, task.GuestOwnerUUID, admitted.Metadata.OwnerUUID)
		assert.Equal(t, "node-01", admitted.Metadata.Node)

		assert.Equal(t, task.KindJob, admitted.Spec.Kind)
		assert.Equal(t, network.PolicyIsolated, admitted.Spec.NetworkPolicy, "the default policy")
		assert.Equal(t, []port.Port{3000, 8080}, admitted.Spec.Ports, "sorted, each once")
		require.NotNil(t, admitted.Spec.MaxRetries)
		assert.Equal(t, 0, *admitted.Spec.MaxRetries)

		assert.Equal(t, taskKind.Created, admitted.Status.State)
		assert.Equal(t, taskKind.Running, admitted.Status.Expected)
		assert.Zero(t, admitted.Metadata.Lifetime, "its ttl is its run's, counted from when it starts")
	})

	t.Run("one that names no kind is a job, and a service is worth a service's retries", func(t *testing.T) {
		t.Parallel()

		asked := snippet()
		asked.Spec.Kind = ""
		asked.Spec.MaxRetries = nil

		admitted, _, err := strategy(nil).tasks.Admit(ctx, asked)
		require.NoError(t, err)
		assert.Equal(t, task.KindJob, admitted.Spec.Kind)
		assert.Equal(t, 0, *admitted.Spec.MaxRetries)

		asked.Spec.Kind, asked.Spec.TTL = task.KindService, 0

		admitted, _, err = strategy(nil).tasks.Admit(ctx, asked)
		require.NoError(t, err)
		assert.Equal(t, task.DefaultMaxRetries(task.KindService), *admitted.Spec.MaxRetries)
	})

	t.Run("one admitted while no node has spoken lately is placed when it is run", func(t *testing.T) {
		t.Parallel()

		admitted, invalid, err := strategy([]node.Node{spoke("node-01", time.Minute)}).tasks.Admit(ctx, snippet())
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Empty(t, admitted.Metadata.Node)
	})

	t.Run("a slug a task or a vm holds is not given", func(t *testing.T) {
		t.Parallel()

		calls := 0
		f := strategy(nil)
		f.tasks.Slugs = []slugs.Taken{func(context.Context, string) (bool, error) {
			calls++

			return calls == 1, nil
		}}

		_, _, err := f.tasks.Admit(ctx, snippet())
		require.NoError(t, err)
		assert.Equal(t, 2, calls, "the first slug was taken, and another was made")
	})

	for name, tt := range map[string]struct {
		change func(t *taskKind.Task)
		field  string
		code   string
	}{
		"no name":                     {change: func(t *taskKind.Task) { t.Metadata.Name = " " }, field: "name", code: "required_field"},
		"no image":                    {change: func(t *taskKind.Task) { t.Spec.Image = "" }, field: "image", code: "required_field"},
		"no cpu":                      {change: func(t *taskKind.Task) { t.Spec.Limits.CPU = 0 }, field: "limits.cpu", code: "required_field"},
		"no memory":                   {change: func(t *taskKind.Task) { t.Spec.Limits.Memory = 0 }, field: "limits.memory", code: "required_field"},
		"less memory than a vm takes": {change: func(t *taskKind.Task) { t.Spec.Limits.Memory = task.MinMemory - 1 }, field: "limits.memory", code: "memory_below_minimum"},
		"no disk":                     {change: func(t *taskKind.Task) { t.Spec.Limits.Disk = 0 }, field: "limits.disk", code: "required_field"},
		"a kind nobody knows":         {change: func(t *taskKind.Task) { t.Spec.Kind = "cron" }, field: "kind", code: "invalid_value"},
		"fewer retries than none": {change: func(t *taskKind.Task) {
			retries := -2
			t.Spec.MaxRetries = &retries
		}, field: "max_retries", code: "invalid_value"},
		"a ttl before nothing":           {change: func(t *taskKind.Task) { t.Spec.TTL = -time.Second }, field: "ttl", code: "invalid_value"},
		"a ttl for a service":            {change: func(t *taskKind.Task) { t.Spec.Kind = task.KindService }, field: "ttl", code: "ttl_requires_a_job"},
		"a policy nobody knows":          {change: func(t *taskKind.Task) { t.Spec.NetworkPolicy = "host" }, field: "network_policy", code: "invalid_network_policy"},
		"port zero":                      {change: func(t *taskKind.Task) { t.Spec.Ports = []port.Port{0} }, field: "ports", code: "invalid_value"},
		"ports with no network to serve": {change: func(t *taskKind.Task) { t.Spec.NetworkPolicy = network.PolicyNone }, field: "ports", code: "ports_require_network"},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			asked := snippet()
			tt.change(&asked)

			_, invalid, err := strategy([]node.Node{spoke("node-01", time.Second)}).tasks.Admit(ctx, asked)
			require.NoError(t, err)
			assert.Equal(t, tt.code, invalid[tt.field], "%v", invalid)
		})
	}
}

// in is a snippet admitted on node-01, as it is now.
func in(state kind.State, expected kind.State, change ...func(t *taskKind.Task)) taskKind.Task {
	t := snippet()
	t.Metadata.UUID = "task-uuid"
	t.Metadata.Node = "node-01"
	t.Status.State = state
	t.Status.Expected = expected
	t.Status.Since = now.Add(-time.Second)

	for _, c := range change {
		c(&t)
	}

	return t
}

// started has a task's run start ago.
func started(ago time.Duration) func(t *taskKind.Task) {
	return func(t *taskKind.Task) {
		t.Status.Run = &taskKind.Run{Name: t.Metadata.Name, Kind: task.KindJob, StartedAt: now.Add(-ago)}
	}
}

func service(t *taskKind.Task) {
	retries := 3

	t.Spec.Kind = task.KindService
	t.Spec.TTL = 0
	t.Spec.MaxRetries = &retries
}

func retried(times int) func(t *taskKind.Task) {
	return func(t *taskKind.Task) { t.Status.Retries = times }
}

func TestTasks_Reconcile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		task taskKind.Task
		want string
	}{
		"one admitted is run":                                  {task: in(taskKind.Created, taskKind.Running), want: taskKind.ActionCreate},
		"a job that completed has run to its end, and goes":    {task: in(taskKind.Completed, taskKind.Running), want: taskKind.ActionDelete},
		"and so does one that was stopped":                     {task: in(taskKind.Completed, taskKind.Stopped), want: taskKind.ActionDelete},
		"and one that stopped":                                 {task: in(taskKind.Stopped, taskKind.Stopped), want: taskKind.ActionDelete},
		"and one that failed, with no retries left":            {task: in(taskKind.Failed, taskKind.Running), want: taskKind.ActionDelete},
		"a job that failed and is worth another attempt":       {task: in(taskKind.Failed, taskKind.Running, func(t *taskKind.Task) { retries := 1; t.Spec.MaxRetries = &retries }), want: taskKind.ActionCreate},
		"but not once its retries are spent":                   {task: in(taskKind.Failed, taskKind.Running, func(t *taskKind.Task) { retries := 1; t.Spec.MaxRetries = &retries }, retried(1)), want: taskKind.ActionDelete},
		"a job running within its ttl is left to run":          {task: in(taskKind.Running, taskKind.Running, started(59*time.Second))},
		"one past it is killed":                                {task: in(taskKind.Running, taskKind.Running, started(61*time.Second)), want: taskKind.ActionKill},
		"counted from when its run started, not before":        {task: in(taskKind.Running, taskKind.Running, func(t *taskKind.Task) { t.Metadata.CreatedAt = now.Add(-time.Hour) }, started(time.Second))},
		"one that never started is never killed for it":        {task: in(taskKind.Running, taskKind.Running, func(t *taskKind.Task) { t.Metadata.CreatedAt = now.Add(-time.Hour) })},
		"nor one that may run as long as it likes":             {task: in(taskKind.Running, taskKind.Running, started(time.Hour), func(t *taskKind.Task) { t.Spec.TTL = 0 })},
		"one running while it was expected stopped is stopped": {task: in(taskKind.Running, taskKind.Stopped, started(time.Second)), want: taskKind.ActionStop},
		"a job whose node fell silent is left to its node":     {task: in(taskKind.Failed, taskKind.Running, func(t *taskKind.Task) { t.Status.Reason = reconcileResources.ReasonNodeLost })},
		"until it is given up on": {task: in(taskKind.Failed, taskKind.Running, func(t *taskKind.Task) {
			t.Status.Reason, t.Status.Since = reconcileResources.ReasonNodeLost, now.Add(-LostAfter)
		}), want: taskKind.ActionDelete},
		"a service that failed is run again":         {task: in(taskKind.Failed, taskKind.Running, service), want: taskKind.ActionCreate},
		"and one that stopped unasked":               {task: in(taskKind.Stopped, taskKind.Running, service), want: taskKind.ActionCreate},
		"until its retries are spent":                {task: in(taskKind.Failed, taskKind.Running, service, retried(3))},
		"forever when it is worth as many":           {task: in(taskKind.Failed, taskKind.Running, service, retried(300), func(t *taskKind.Task) { forever := task.RetryForever; t.Spec.MaxRetries = &forever }), want: taskKind.ActionCreate},
		"a service stopped as asked is left stopped": {task: in(taskKind.Stopped, taskKind.Stopped, service)},
		"a service whose node fell silent is left to its node": {task: in(taskKind.Failed, taskKind.Running, service, func(t *taskKind.Task) {
			t.Status.Reason, t.Status.Since = reconcileResources.ReasonNodeLost, now.Add(-time.Hour)
		})},
		"a service that runs is left to run": {task: in(taskKind.Running, taskKind.Running, service)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			intents, err := strategy(nil).tasks.Reconcile(ctx, tt.task)
			require.NoError(t, err)

			if len(tt.want) == 0 {
				assert.Empty(t, intents)

				return
			}

			require.Len(t, intents, 1)
			assert.Equal(t, tt.want, intents[0].Action)
			assert.NotEmpty(t, intents[0].Reason)
		})
	}
}

func TestTasks_Prepare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a task on no node is placed before it is run", func(t *testing.T) {
		t.Parallel()

		unplaced := in(taskKind.Created, taskKind.Running, func(t *taskKind.Task) { t.Metadata.Node = "" })

		prepared, invalid, err := strategy([]node.Node{spoke("node-02", time.Second)}).tasks.Prepare(ctx, unplaced, taskKind.ActionCreate, nil)
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, "node-02", prepared.Metadata.Node)
		assert.Zero(t, prepared.Status.Retries, "its first attempt")
	})

	t.Run("and left as it was while no node has spoken", func(t *testing.T) {
		t.Parallel()

		unplaced := in(taskKind.Failed, taskKind.Running, func(t *taskKind.Task) { t.Metadata.Node = "" })

		prepared, _, err := strategy([]node.Node{spoke("node-02", time.Minute)}).tasks.Prepare(ctx, unplaced, taskKind.ActionCreate, nil)
		require.NoError(t, err)
		assert.Equal(t, unplaced, prepared, "no attempt is counted that is not made")
	})

	t.Run("one run again after it ended is its next attempt, on the node it was on", func(t *testing.T) {
		t.Parallel()

		prepared, _, err := strategy(nil).tasks.Prepare(ctx, in(taskKind.Failed, taskKind.Running, service, retried(1)), taskKind.ActionCreate, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, prepared.Status.Retries)
		assert.Equal(t, "node-01", prepared.Metadata.Node)
	})

	t.Run("its log goes as its delete is sent", func(t *testing.T) {
		t.Parallel()

		f := strategy(nil)
		require.NoError(t, f.logs.Append(ctx, []task.Log{{TaskUUID: "task-uuid", LogLine: task.LogLine{Content: "hello", At: now}}}))

		prepared, _, err := f.tasks.Prepare(ctx, in(taskKind.Completed, taskKind.Running), taskKind.ActionDelete, nil)
		require.NoError(t, err)
		assert.Equal(t, in(taskKind.Completed, taskKind.Running), prepared)
		assert.Zero(t, f.logs.Count("task-uuid"))
	})

	t.Run("and anything else is sent as it is", func(t *testing.T) {
		t.Parallel()

		running := in(taskKind.Running, taskKind.Running, started(time.Second))

		prepared, _, err := strategy(nil).tasks.Prepare(ctx, running, taskKind.ActionKill, nil)
		require.NoError(t, err)
		assert.Equal(t, running, prepared)
	})
}

func TestTasks_Apply(t *testing.T) {
	t.Parallel()

	_, _, err := strategy(nil).tasks.Apply(context.Background(), in(taskKind.Running, taskKind.Running), "rename", nil)
	assert.True(t, errors.Is(err, kind.ErrUnknownAction))
}

// The strategy is bound to the kind as the control plane binds it.
func TestBinding(t *testing.T) {
	t.Parallel()

	binding := kind.BindControlPlane[taskKind.Spec, taskKind.Status](taskKind.Descriptor(), strategy(nil).tasks)

	_, extended := binding.Extras()
	assert.False(t, extended, "a task's listings are its records alone")

	raw, err := kind.Encode(snippet())
	require.NoError(t, err)

	admitted, invalid, err := binding.Admit(context.Background(), raw)
	require.NoError(t, err)
	require.Empty(t, invalid)
	assert.Equal(t, taskKind.Name, admitted.Kind)

	_, invalid, err = binding.Admit(context.Background(), kind.Raw{Kind: taskKind.Name, Metadata: raw.Metadata, Spec: []byte(`{"limits": {"cpu": 1}}`)})
	require.NoError(t, err)
	assert.Equal(t, domain.ValidationErrors{
		"image":         "required_field",
		"limits.memory": "required_field",
		"limits.disk":   "required_field",
	}, invalid)
}
