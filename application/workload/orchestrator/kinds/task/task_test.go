package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

// snippet is a task as the control plane records one of the code runner's:
// a job of the guest's, placed on this node.
func snippet(uuid string, change ...func(t *taskKind.Task)) taskKind.Task {
	none := 0

	t := taskKind.Task{
		Kind: taskKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "request-" + uuid,
			Slug:      "request-" + uuid + "-abcde",
			OwnerUUID: task.GuestOwnerUUID,
			Node:      nodeName,
		},
		Spec: taskKind.Spec{
			Kind:          task.KindJob,
			Image:         "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
			Command:       []string{"--timeout", "30", "console.log(1)"},
			NetworkPolicy: network.PolicyIsolated,
			TTL:           time.Minute,
			Limits:        taskKind.Limits{CPU: 1.5, Memory: 200 << 20, Disk: 100 << 20},
			MaxRetries:    &none,
		},
		Status: taskKind.Status{Status: kind.Status{State: taskKind.Scheduled, Expected: taskKind.Running}},
	}

	for _, c := range change {
		c(&t)
	}

	return t
}

// live is a snippet somebody is watching, which serves a port.
func live(t *taskKind.Task) {
	t.Spec.Interactive = true
	t.Spec.Ports = []port.Port{3000}
	t.Spec.TTL = 2 * time.Minute
}

type fixture struct {
	node    *Node
	engine  *memory.Engine
	runtime *vmruntime.Runtime
}

func onNode(options ...memory.Option) fixture {
	engine := memory.New(options...)
	runtime := vmruntime.New(engine, slog.New(slog.DiscardHandler))

	return fixture{node: New(runtime, nodeName), engine: engine, runtime: runtime}
}

// run is the one run of a task the engine holds.
func (f fixture) run(t *testing.T, uuid string) task.Execution {
	t.Helper()

	runs, err := f.runtime.Of(context.Background(), uuid)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	return runs[0]
}

func (f fixture) created(t *testing.T, tk taskKind.Task) kind.Outcome[taskKind.Status] {
	t.Helper()

	outcome, err := f.node.Execute(context.Background(), tk, taskKind.ActionCreate, nil)
	require.NoError(t, err)

	return outcome
}

func TestNode_create(t *testing.T) {
	t.Parallel()

	t.Run("a task is run in a vm of its own, labelled with what it is", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		outcome := f.created(t, snippet("task-1", live))

		run := f.run(t, "task-1")
		spec, err := f.engine.Spec(run.ID)
		require.NoError(t, err)

		assert.Equal(t, "ghcr.io/tarhche/code-runner:nodejs-22.14-latest", spec.Image, "a machine's, as it says")
		assert.Equal(t, []string{"--timeout", "30", "console.log(1)"}, spec.Command)
		assert.Equal(t, vm.Resources{CPUs: 2, Memory: 200 << 20, Disk: 100 << 20}, spec.Resources, "whole vCPUs, rounded up, and bytes as they were asked for")
		assert.Equal(t, []port.Port{3000}, spec.Ports)
		assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, spec.Network)

		assert.Equal(t, vm.PurposeTask, spec.Labels[vm.LabelPurpose])
		assert.Equal(t, "task-1", spec.Labels[vm.LabelTask])
		assert.Equal(t, "request-task-1-abcde", spec.Labels[vm.LabelSlug])
		assert.Equal(t, task.GuestOwnerUUID, spec.Labels[vm.LabelOwner])

		assert.Equal(t, task.Execution{
			ID:          run.ID,
			Name:        "request-task-1-abcde",
			TaskUUID:    "task-1",
			TaskName:    "request-task-1",
			Slug:        "request-task-1-abcde",
			Kind:        task.KindJob,
			NodeName:    nodeName,
			OwnerUUID:   task.GuestOwnerUUID,
			Interactive: true,
			TTL:         2 * time.Minute,
			Status:      task.StatusRunning,
			Image:       "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
			StartedAt:   run.StartedAt,
			ExposedPorts: port.PortSet{
				3000: struct{}{},
			},
			PortBindings: port.PortMap{
				3000: []port.PortBinding{{HostIP: "vmhost", HostPort: 20000}},
			},
		}, run, "what its node reads back off it")

		require.NotNil(t, outcome.Status.Run)
		assert.Equal(t, taskKind.Running, outcome.Status.State)

		config := snippet("task-1", live).Spec.Config()
		assert.Equal(t, &config, outcome.Status.Applied, "it says what it made the run with")

		assert.Equal(t, &taskKind.Run{
			ID:          run.ID,
			Name:        "request-task-1",
			Slug:        "request-task-1-abcde",
			Kind:        task.KindJob,
			Interactive: true,
			Ports:       []port.Port{3000},
			StartedAt:   run.StartedAt,
			Deadline:    run.StartedAt.Add(2 * time.Minute),
			Endpoints:   []taskKind.Endpoint{{Port: 3000, Address: "vmhost:20000"}},
		}, outcome.Status.Run)
	})

	t.Run("one already running is left to run", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		f.created(t, snippet("task-1"))
		first := f.run(t, "task-1")

		again := f.created(t, snippet("task-1"))

		assert.Equal(t, first.ID, f.run(t, "task-1").ID, "the same run")
		assert.Equal(t, taskKind.Running, again.Status.State)
		assert.Nil(t, again.Status.Applied, "and nothing is said of what it was made with, which this create did not give it")
	})

	t.Run("its next attempt is a run anew, and what is left of the last goes", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		f.created(t, snippet("task-1"))
		first := f.run(t, "task-1")
		require.NoError(t, f.engine.Exit(first.ID, 1))

		outcome := f.created(t, snippet("task-1", func(t *taskKind.Task) {
			t.Status.State = taskKind.Failed
			t.Status.Retries = 1
		}))

		second := f.run(t, "task-1")
		assert.NotEqual(t, first.ID, second.ID)
		assert.Equal(t, 1, second.Attempt)
		assert.Equal(t, 1, outcome.Status.Run.Attempt)
		assert.Equal(t, taskKind.Running, outcome.Status.State)
	})

	t.Run("one asked for twice takes the run of its attempt that is there", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		f.created(t, snippet("task-1"))
		first := f.run(t, "task-1")
		require.NoError(t, f.engine.Exit(first.ID, 0))

		again := f.created(t, snippet("task-1"))

		assert.Equal(t, first.ID, f.run(t, "task-1").ID, "an ended run is not run again")
		assert.Equal(t, taskKind.Completed, again.Status.State)
	})

	t.Run("one that cannot be run fails, saying why and whose request it answers", func(t *testing.T) {
		t.Parallel()

		f := onNode(memory.WithCapacity(1, 100<<20, 1<<30))

		outcome, err := f.node.Execute(context.Background(), snippet("task-1", live), taskKind.ActionCreate, nil)
		require.Error(t, err)

		assert.Equal(t, taskKind.Failed, outcome.Status.State)
		assert.Equal(t, err.Error(), outcome.Status.Reason)
		assert.Equal(t, &taskKind.Run{Name: "request-task-1", Slug: "request-task-1-abcde", Kind: task.KindJob, Interactive: true}, outcome.Status.Run)

		runs, err := f.runtime.Of(context.Background(), "task-1")
		require.NoError(t, err)
		assert.Empty(t, runs)
	})
}

func TestNode_stop(t *testing.T) {
	t.Parallel()

	for _, action := range []string{taskKind.ActionStop, taskKind.ActionKill} {
		t.Run("a running job "+action+"ped ends, as a job cut short completes", func(t *testing.T) {
			t.Parallel()

			f := onNode()
			f.created(t, snippet("task-1"))

			outcome, err := f.node.Execute(context.Background(), snippet("task-1"), action, nil)
			require.NoError(t, err)
			assert.Equal(t, taskKind.Completed, outcome.Status.State)

			assert.Equal(t, task.StatusExited, f.run(t, "task-1").Status)
		})
	}

	t.Run("one this node holds nothing of has stopped", func(t *testing.T) {
		t.Parallel()

		outcome, err := onNode().node.Execute(context.Background(), snippet("task-1"), taskKind.ActionStop, nil)
		require.NoError(t, err)
		assert.Equal(t, taskKind.Stopped, outcome.Status.State)
	})
}

func TestNode_reconfigure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// changed is a live snippet changed while it runs: renamed, serving
	// another port beside its own, reaching out, and given more of
	// everything.
	changed := func(t *taskKind.Task) {
		live(t)
		t.Metadata.Name = "renamed"
		t.Spec.Ports = []port.Port{3000, 8080}
		t.Spec.NetworkPolicy = network.PolicyPublic
		t.Spec.Limits = taskKind.Limits{CPU: 3, Memory: 300 << 20, Disk: 200 << 20}
		t.Status.State = taskKind.Restarting
	}

	t.Run("a running task's run is made afresh with what the task now has, and runs its program again", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1", live))

		before := f.run(t, "task-1")
		require.NoError(t, f.engine.Log(before.ID, vm.LogSourceMain, "written before"))

		time.Sleep(time.Millisecond)

		outcome, err := f.node.Execute(ctx, snippet("task-1", changed), taskKind.ActionReconfigure, nil)
		require.NoError(t, err)

		after := f.run(t, "task-1")
		assert.Equal(t, before.ID, after.ID, "the same run")
		assert.Equal(t, task.StatusRunning, after.Status)
		assert.True(t, after.StartedAt.After(before.StartedAt), "started again")

		spec, err := f.engine.Spec(after.ID)
		require.NoError(t, err)
		assert.Equal(t, vm.Resources{CPUs: 3, Memory: 300 << 20, Disk: 200 << 20}, spec.Resources)
		assert.Equal(t, []port.Port{3000, 8080}, spec.Ports)
		assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}, spec.Network)
		assert.Equal(t, []string{"--timeout", "30", "console.log(1)"}, spec.Command, "it runs what it ran")
		assert.Equal(t, "request-task-1", spec.Labels["workload.task.name"], "and is labelled as it was made")

		assert.Equal(t, taskKind.Running, outcome.Status.State)
		require.NotNil(t, outcome.Status.Run)
		assert.Equal(t, "request-task-1", outcome.Status.Run.Name, "whose reader it still answers")
		assert.Equal(t, after.StartedAt, outcome.Status.Run.StartedAt)
		assert.Equal(t, after.StartedAt.Add(2*time.Minute), outcome.Status.Run.Deadline, "its deadline counted from its new start")
		assert.Equal(t, []port.Port{3000, 8080}, outcome.Status.Run.Ports)

		config := snippet("task-1", changed).Spec.Config()
		assert.Equal(t, &config, outcome.Status.Applied, "it says what it gave the run")
	})

	t.Run("one whose run ended meanwhile is given it all the same, and has ended still", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1", live))
		require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 0))

		outcome, err := f.node.Execute(ctx, snippet("task-1", changed), taskKind.ActionReconfigure, nil)
		require.NoError(t, err)
		assert.Equal(t, taskKind.Completed, outcome.Status.State)
		assert.NotNil(t, outcome.Status.Applied)

		spec, err := f.engine.Spec(f.run(t, "task-1").ID)
		require.NoError(t, err)
		assert.Equal(t, []port.Port{3000, 8080}, spec.Ports)
	})

	t.Run("one this node holds no run of has nothing to give it to", func(t *testing.T) {
		t.Parallel()

		_, err := onNode().node.Execute(ctx, snippet("task-1", changed), taskKind.ActionReconfigure, nil)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("while it is carried out the task is restarting, whatever its run is seen doing", func(t *testing.T) {
		t.Parallel()

		engine := &halting{Engine: memory.New(), down: make(chan struct{}), release: make(chan struct{})}
		node := New(vmruntime.New(engine, slog.New(slog.DiscardHandler)), nodeName)

		_, err := node.Execute(ctx, snippet("task-1", live), taskKind.ActionCreate, nil)
		require.NoError(t, err)

		done := make(chan error, 1)
		go func() {
			_, err := node.Execute(ctx, snippet("task-1", changed), taskKind.ActionReconfigure, nil)
			done <- err
		}()

		<-engine.down

		report, err := node.State(ctx)
		require.NoError(t, err)
		require.Len(t, report.Instances, 1)
		assert.Equal(t, taskKind.Restarting, report.Instances[0].Status.State, "not completed, which its vm, down for now, would say")
		assert.Zero(t, report.Instances[0].Status.Run.ExitCode)

		close(engine.release)
		require.NoError(t, <-done)

		report, err = node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, taskKind.Running, report.Instances[0].Status.State, "and running once it is up again")
	})

	t.Run("and so it is in a look taken while it was, and read once it was done", func(t *testing.T) {
		t.Parallel()

		engine := &halting{Engine: memory.New(), down: make(chan struct{}), release: make(chan struct{})}
		runtime := &pausing{Runtime: vmruntime.New(engine, slog.New(slog.DiscardHandler)), listed: make(chan struct{}), release: make(chan struct{})}
		node := New(runtime, nodeName)

		_, err := node.Execute(ctx, snippet("task-1", live), taskKind.ActionCreate, nil)
		require.NoError(t, err)

		reconfigured := make(chan error, 1)
		go func() {
			_, err := node.Execute(ctx, snippet("task-1", changed), taskKind.ActionReconfigure, nil)
			reconfigured <- err
		}()

		<-engine.down

		looked := make(chan kind.Report[taskKind.Status], 1)
		go func() {
			report, err := node.State(ctx)
			assert.NoError(t, err)
			looked <- report
		}()

		// the look saw its run down, and is read only once it is up again.
		<-runtime.listed
		close(engine.release)
		require.NoError(t, <-reconfigured)
		close(runtime.release)

		report := <-looked
		require.Len(t, report.Instances, 1)
		assert.Equal(t, taskKind.Restarting, report.Instances[0].Status.State, "its reader is not told it ended")

		report, err = node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, taskKind.Running, report.Instances[0].Status.State, "and a look taken after it was done says what its run does")
	})
}

// halting is an engine whose first reconfigure stops the instance it is
// asked of, and keeps it down until it is let go of, before it boots it
// again with what it was given: a reconfigure caught halfway through, as an
// engine that cannot change a running VM carries one out.
type halting struct {
	*memory.Engine

	down    chan struct{}
	release chan struct{}
	once    sync.Once
}

func (h *halting) Reconfigure(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	halted := false

	h.once.Do(func() {
		halted = true

		if err := h.Engine.Stop(ctx, spec.ID); err != nil {
			panic(err)
		}

		close(h.down)
		<-h.release
	})

	if !halted {
		return h.Engine.Reconfigure(ctx, spec)
	}

	if _, err := h.Engine.Reconfigure(ctx, spec); err != nil {
		return vm.Instance{}, err
	}

	if err := h.Engine.Start(ctx, spec.ID); err != nil {
		return vm.Instance{}, err
	}

	return h.Engine.Inspect(ctx, spec.ID)
}

// pausing is a runtime whose first look at what a node holds is taken, and
// then held back until it is let go of: a look read long after it was taken.
type pausing struct {
	task.Runtime

	listed  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *pausing) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	held, err := p.Runtime.OnNode(ctx, nodeName)

	p.once.Do(func() {
		close(p.listed)
		<-p.release
	})

	return held, err
}

// beat is ctx asking for what the node holds on one of its beats, as its
// heartbeat asks for it (kind.WithBeat), rather than between them.
func beat(ctx context.Context) context.Context {
	return kind.WithBeat(ctx, time.Now())
}

// clock is the time as a test moves it on.
type clock struct {
	lock sync.Mutex
	at   time.Time
}

func (c *clock) now() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.at
}

func (c *clock) pass(d time.Duration) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.at = c.at.Add(d)
}

// failingStats is a runtime that cannot sample anything.
type failingStats struct {
	task.Runtime
}

func (failingStats) Stats(context.Context, string) (vm.Stats, error) {
	return vm.Stats{}, errors.New("the engine is busy")
}

// counting is a runtime that counts the samples it is asked for.
type counting struct {
	task.Runtime

	lock  sync.Mutex
	asked int
}

func (c *counting) Stats(ctx context.Context, executionID string) (vm.Stats, error) {
	c.lock.Lock()
	c.asked++
	c.lock.Unlock()

	return c.Runtime.Stats(ctx, executionID)
}

func (c *counting) calls() int {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.asked
}

// stalling is a runtime whose samples are not answered until they are let go
// of, as an engine slow to answer takes them, and which says the most it was
// asked for at once.
type stalling struct {
	task.Runtime

	release chan struct{}

	lock   sync.Mutex
	asking int
	peak   int
}

func (s *stalling) Stats(context.Context, string) (vm.Stats, error) {
	s.lock.Lock()
	s.asking++
	s.peak = max(s.peak, s.asking)
	s.lock.Unlock()

	defer func() {
		s.lock.Lock()
		s.asking--
		s.lock.Unlock()
	}()

	<-s.release

	return vm.Stats{}, errors.New("answered too late")
}

// most is the most samples it was asked for at once.
func (s *stalling) most() int {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.peak
}

func TestNode_delete(t *testing.T) {
	t.Parallel()

	f := onNode()
	f.created(t, snippet("task-1"))

	_, err := f.node.Execute(context.Background(), snippet("task-1"), taskKind.ActionDelete, nil)
	require.NoError(t, err)

	runs, err := f.runtime.Of(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Empty(t, runs)

	_, err = f.node.Execute(context.Background(), snippet("task-1"), taskKind.ActionDelete, nil)
	assert.NoError(t, err, "one that is gone is deleted already")
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("is looked at between beats, as often as a snippet's reader was always told what it did", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 300*time.Millisecond, onNode().node.Prompt())
	})

	t.Run("every task held, its run with what a job has written", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		f.created(t, snippet("task-2"))
		completed := f.run(t, "task-2")
		require.NoError(t, f.engine.Log(completed.ID, vm.LogSourceMain, "hello"))
		require.NoError(t, f.engine.Log(completed.ID, vm.LogSourceKernel, "booting"))
		require.NoError(t, f.engine.Log(completed.ID, vm.LogSourceMain, "bye"))
		require.NoError(t, f.engine.Exit(completed.ID, 0))

		f.created(t, snippet("task-3"))
		failing := f.run(t, "task-3")
		require.NoError(t, f.engine.Log(failing.ID, vm.LogSourceMain, "⏰ Execution timed out after 30 seconds"))
		require.NoError(t, f.engine.Exit(failing.ID, 124))

		f.created(t, snippet("task-1", live))
		watched := f.run(t, "task-1")

		f.created(t, snippet("task-4", func(t *taskKind.Task) {
			t.Spec.Kind = task.KindService
			t.Spec.TTL = 0
		}))
		serving := f.run(t, "task-4")
		require.NoError(t, f.engine.Log(serving.ID, vm.LogSourceMain, "listening"))

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		require.Len(t, report.Instances, 4)

		uuids := make([]string, len(report.Instances))
		for i, instance := range report.Instances {
			uuids[i] = instance.UUID
		}

		assert.Equal(t, []string{"task-1", "task-2", "task-3", "task-4"}, uuids)

		assert.Equal(t, taskKind.Running, report.Instances[0].Status.State)
		assert.Equal(t, watched.StartedAt.Add(2*time.Minute), report.Instances[0].Status.Run.Deadline)
		assert.Equal(t, []taskKind.Endpoint{{Port: 3000, Address: "vmhost:20000"}}, report.Instances[0].Status.Run.Endpoints)
		assert.True(t, report.Instances[0].Status.Run.Interactive)

		assert.Equal(t, taskKind.Completed, report.Instances[1].Status.State)
		assert.Equal(t, "hello\nbye\n", report.Instances[1].Status.Run.Output, "what its program wrote, and nothing of its vm's")
		assert.Empty(t, report.Instances[1].Status.Run.Endpoints, "a run that has ended serves nothing")

		assert.Equal(t, taskKind.Failed, report.Instances[2].Status.State)
		assert.Equal(t, 124, report.Instances[2].Status.Run.ExitCode)
		assert.Equal(t, "the task failed", report.Instances[2].Status.Reason)
		assert.Equal(t, "⏰ Execution timed out after 30 seconds\n", report.Instances[2].Status.Run.Output)

		assert.Equal(t, taskKind.Running, report.Instances[3].Status.State)
		assert.Empty(t, report.Instances[3].Status.Run.Output, "a service's log is shipped a line at a time")
	})

	t.Run("an ended job's output is read once", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1"))

		run := f.run(t, "task-1")
		require.NoError(t, f.engine.Log(run.ID, vm.LogSourceMain, "hello"))
		require.NoError(t, f.engine.Exit(run.ID, 0))

		first, err := f.node.State(ctx)
		require.NoError(t, err)

		require.NoError(t, f.engine.Log(run.ID, vm.LogSourceMain, "written after it ended"))

		second, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, first.Instances[0].Status.Run.Output, second.Instances[0].Status.Run.Output)

		_, err = f.node.Execute(ctx, snippet("task-1"), taskKind.ActionDelete, nil)
		require.NoError(t, err)

		gone, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Empty(t, gone.Instances)
		assert.Empty(t, f.node.outputs, "what was kept of a run goes with it")
	})

	t.Run("and read again once its run is started again, which writes something else", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1"))

		run := f.run(t, "task-1")
		require.NoError(t, f.engine.Log(run.ID, vm.LogSourceMain, "the first time"))
		require.NoError(t, f.engine.Exit(run.ID, 0))

		first, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, "the first time\n", first.Instances[0].Status.Run.Output)

		time.Sleep(time.Millisecond)

		// started again, as a reconfigure of a run that ended does when it
		// next boots, and ended again having written more.
		require.NoError(t, f.engine.Restart(ctx, run.ID))
		require.NoError(t, f.engine.Log(run.ID, vm.LogSourceMain, "the second time"))
		require.NoError(t, f.engine.Exit(run.ID, 0))

		second, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, "the first time\nthe second time\n", second.Instances[0].Status.Run.Output)
	})

	t.Run("a run of no task, and one of another node, are nobody's here", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		_, err := f.engine.Create(ctx, vm.Spec{ID: "stray", Image: "busybox", Labels: map[string]string{vm.LabelPurpose: vm.PurposeTask}})
		require.NoError(t, err)

		// a run made by another node, which shares this one's engine.
		elsewhere := New(f.runtime, "elsewhere")
		_, err = elsewhere.Execute(ctx, snippet("task-1", func(t *taskKind.Task) { t.Metadata.Node = "elsewhere" }), taskKind.ActionCreate, nil)
		require.NoError(t, err)

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Empty(t, report.Instances)

		theirs, err := elsewhere.State(ctx)
		require.NoError(t, err)
		require.Len(t, theirs.Instances, 1)
	})

	t.Run("a task's running run speaks for it, before an earlier attempt that ended", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1"))
		require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 1))

		// its next attempt made beside it, as a node that was away might.
		next := executionOf(snippet("task-1"), 1, nodeName)
		_, err := f.runtime.Create(ctx, &next)
		require.NoError(t, err)

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		require.Len(t, report.Instances, 1)
		assert.Equal(t, taskKind.Running, report.Instances[0].Status.State)
		assert.Equal(t, 1, report.Instances[0].Status.Run.Attempt)
	})

	t.Run("a run says the ports its vm publishes, which the ingress reaches it on, whether or not they are up", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		f.created(t, snippet("task-1", live))
		require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 0))

		f.created(t, snippet("task-2", live, func(t *taskKind.Task) { t.Spec.NetworkPolicy = network.PolicyNone }))

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		require.Len(t, report.Instances, 2)

		ended := report.Instances[0].Status.Run
		assert.Equal(t, []port.Port{3000}, ended.Ports, "a run that has ended still says what it serves")
		assert.Empty(t, ended.Endpoints, "and serves nothing now")

		assert.Equal(t, taskKind.Running, report.Instances[1].Status.State)
		assert.Empty(t, report.Instances[1].Status.Run.Ports, "one whose policy lets nothing in publishes nothing")
	})

	t.Run("a running task's run is sampled on the node's beat, as a vm is, and one that ended uses nothing", func(t *testing.T) {
		t.Parallel()

		f := onNode()

		f.created(t, snippet("task-1", live))
		require.NoError(t, f.engine.SetStats(f.run(t, "task-1").ID, vm.Stats{CPUPercent: 130, MemoryUsed: 64 << 20, DiskUsed: 10 << 20, NetworkRx: 1, NetworkTx: 2}))

		f.created(t, snippet("task-2"))
		require.NoError(t, f.engine.Exit(f.run(t, "task-2").ID, 0))

		report, err := f.node.State(beat(ctx))
		require.NoError(t, err)
		require.Len(t, report.Instances, 2)

		running := report.Instances[0].Status.Stats
		require.NotNil(t, running, "a running one is sampled")
		assert.Equal(t, 100.0, running.CPUPercent, "its cpu held to 0 to 100, as a vm's is")
		assert.Equal(t, uint64(64<<20), running.MemoryUsed)
		assert.Equal(t, uint64(200<<20), running.MemoryLimit, "of the memory its vm was given")
		assert.Equal(t, uint64(10<<20), running.DiskUsed)
		assert.Equal(t, uint64(100<<20), running.DiskTotal, "and of its disk")
		assert.Equal(t, uint64(1), running.NetworkRx)
		assert.Equal(t, uint64(2), running.NetworkTx)
		assert.False(t, running.SampledAt.IsZero())

		assert.Nil(t, report.Instances[1].Status.Stats, "one that has ended is not sampled")

		require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 0))

		report, err = f.node.State(beat(ctx))
		require.NoError(t, err)
		assert.Nil(t, report.Instances[0].Status.Stats, "nor is one once it has, whatever it used before")
	})

	t.Run("a look between beats never samples a run, and shows the sample the last beat took", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1", live))
		run := f.run(t, "task-1")
		require.NoError(t, f.engine.SetStats(run.ID, vm.Stats{CPUPercent: 10}))

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Nil(t, report.Instances[0].Status.Stats, "nothing is shown before its node has sampled it")

		_, err = f.node.State(beat(ctx))
		require.NoError(t, err)

		require.NoError(t, f.engine.SetStats(run.ID, vm.Stats{CPUPercent: 20}))

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		require.NotNil(t, report.Instances[0].Status.Stats)
		assert.Equal(t, 10.0, report.Instances[0].Status.Stats.CPUPercent, "a look at a prompt is never held up sampling, nor tells of the task again for it")
	})

	t.Run("a running task's run is sampled every other beat, and its sample shown in between", func(t *testing.T) {
		t.Parallel()

		c := &clock{at: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
		f := onNode(memory.WithClock(c.now))
		f.node.now = c.now

		f.created(t, snippet("task-1", live))
		run := f.run(t, "task-1")
		require.NoError(t, f.engine.SetStats(run.ID, vm.Stats{CPUPercent: 10}))

		report, err := f.node.State(beat(ctx))
		require.NoError(t, err)
		assert.Equal(t, 10.0, report.Instances[0].Status.Stats.CPUPercent)

		require.NoError(t, f.engine.SetStats(run.ID, vm.Stats{CPUPercent: 20}))

		c.pass(time.Second)

		report, err = f.node.State(beat(ctx))
		require.NoError(t, err)
		assert.Equal(t, 10.0, report.Instances[0].Status.Stats.CPUPercent, "not sampled again yet")

		c.pass(time.Second)

		report, err = f.node.State(beat(ctx))
		require.NoError(t, err)
		assert.Equal(t, 20.0, report.Instances[0].Status.Stats.CPUPercent)
	})

	t.Run("a sample that cannot be taken shows the last one a while, and then none", func(t *testing.T) {
		t.Parallel()

		c := &clock{at: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
		f := onNode(memory.WithClock(c.now))
		f.node.now = c.now

		f.created(t, snippet("task-1", live))
		require.NoError(t, f.engine.SetStats(f.run(t, "task-1").ID, vm.Stats{CPUPercent: 10}))

		_, err := f.node.State(beat(ctx))
		require.NoError(t, err)

		f.node.runtime = failingStats{Runtime: f.runtime}

		c.pass(5 * time.Second)

		report, err := f.node.State(beat(ctx))
		require.NoError(t, err)
		require.NotNil(t, report.Instances[0].Status.Stats)
		assert.Equal(t, 10.0, report.Instances[0].Status.Stats.CPUPercent)

		c.pass(10 * time.Second)

		report, err = f.node.State(beat(ctx))
		require.NoError(t, err)
		assert.Nil(t, report.Instances[0].Status.Stats)

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		assert.Nil(t, report.Instances[0].Status.Stats, "and between beats as well")
	})

	t.Run("a run being reconfigured uses nothing, and what it used before is not what it uses once it is done", func(t *testing.T) {
		t.Parallel()

		engine := &halting{Engine: memory.New(), down: make(chan struct{}), release: make(chan struct{})}
		sampled := &counting{}
		sampled.Runtime = vmruntime.New(engine, slog.New(slog.DiscardHandler))
		node := New(sampled, nodeName)

		_, err := node.Execute(ctx, snippet("task-1", live), taskKind.ActionCreate, nil)
		require.NoError(t, err)

		runs, err := sampled.Of(ctx, "task-1")
		require.NoError(t, err)
		require.Len(t, runs, 1)
		require.NoError(t, engine.SetStats(runs[0].ID, vm.Stats{CPUPercent: 10}))

		report, err := node.State(beat(ctx))
		require.NoError(t, err)
		require.NotNil(t, report.Instances[0].Status.Stats)

		changed := func(t *taskKind.Task) {
			live(t)
			t.Spec.Limits.Memory = 300 << 20
			t.Status.State = taskKind.Restarting
		}

		done := make(chan error, 1)
		go func() {
			_, err := node.Execute(ctx, snippet("task-1", changed), taskKind.ActionReconfigure, nil)
			done <- err
		}()

		<-engine.down

		asked := sampled.calls()

		report, err = node.State(beat(ctx))
		require.NoError(t, err)
		assert.Equal(t, taskKind.Restarting, report.Instances[0].Status.State)
		assert.Nil(t, report.Instances[0].Status.Stats, "a vm going, and another coming, which is not running yet")
		assert.Equal(t, asked, sampled.calls(), "and is not sampled while it is")

		require.NoError(t, engine.SetStats(runs[0].ID, vm.Stats{CPUPercent: 20}))

		time.Sleep(time.Millisecond)
		close(engine.release)
		require.NoError(t, <-done)

		report, err = node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, taskKind.Running, report.Instances[0].Status.State)
		assert.Nil(t, report.Instances[0].Status.Stats, "what was sampled before is of the vm that went")

		report, err = node.State(beat(ctx))
		require.NoError(t, err)
		require.NotNil(t, report.Instances[0].Status.Stats, "the one made afresh is sampled at the next beat, however soon")
		assert.Equal(t, 20.0, report.Instances[0].Status.Stats.CPUPercent)
		assert.Equal(t, uint64(300<<20), report.Instances[0].Status.Stats.MemoryLimit, "of what it was given since")
	})

	t.Run("runs are sampled a few at once, and a beat that cannot wait any longer goes without them", func(t *testing.T) {
		t.Parallel()

		f := onNode(memory.WithCapacity(64, 64<<30, 64<<30))

		for i := range sampling + 4 {
			f.created(t, snippet(fmt.Sprintf("task-%02d", i), live))
		}

		stalled := &stalling{Runtime: f.runtime, release: make(chan struct{})}
		f.node.runtime = stalled

		defer close(stalled.release)

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Len(t, report.Instances, sampling+4)
		assert.Zero(t, stalled.most(), "a look between beats asks for no sample")

		looking, cancel := context.WithCancel(beat(ctx))
		defer cancel()

		looked := make(chan kind.Report[taskKind.Status], 1)
		go func() {
			report, err := f.node.State(looking)
			assert.NoError(t, err)
			looked <- report
		}()

		require.Eventually(t, func() bool { return stalled.most() == sampling }, 5*time.Second, time.Millisecond)

		// a moment more, in which no more are asked for.
		time.Sleep(20 * time.Millisecond)
		assert.Equal(t, sampling, stalled.most(), "no more than a few at once")

		// the beat can wait no longer.
		cancel()

		select {
		case report := <-looked:
			require.Len(t, report.Instances, sampling+4)

			for _, instance := range report.Instances {
				assert.Nil(t, instance.Status.Stats, "%s goes without what is still being sampled", instance.UUID)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the beat was held up past its time by what was still being sampled")
		}
	})
}

func TestNode_Query(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what a task's run wrote, its last lines as many as are asked for", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1"))

		run := f.run(t, "task-1")
		for _, line := range []string{"one", "two", "three"} {
			require.NoError(t, f.engine.Log(run.ID, vm.LogSourceMain, line))
		}

		answer, err := f.node.Query(ctx, snippet("task-1"), taskKind.ActionLogs, taskKind.LogsPayload{Tail: 2})
		require.NoError(t, err)
		assert.Equal(t, taskKind.Logs{Lines: []string{"two", "three"}, Truncated: true}, answer)

		answer, err = f.node.Query(ctx, snippet("task-1"), taskKind.ActionLogs, taskKind.LogsPayload{})
		require.NoError(t, err)
		assert.Equal(t, taskKind.Logs{Lines: []string{"one", "two", "three"}}, answer)

		_, err = f.node.Query(ctx, snippet("task-9"), taskKind.ActionLogs, taskKind.LogsPayload{})
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("a sample of what its running run uses, as a vm's, its cpu held to 0 to 100", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1", live))
		require.NoError(t, f.engine.SetStats(f.run(t, "task-1").ID, vm.Stats{CPUPercent: 130, MemoryUsed: 64 << 20}))

		answer, err := f.node.Query(ctx, snippet("task-1", live), taskKind.ActionStats, nil)
		require.NoError(t, err)

		stats := answer.(*vmKind.Stats)
		assert.Equal(t, 100.0, stats.CPUPercent)
		assert.Equal(t, uint64(64<<20), stats.MemoryUsed)
		assert.Equal(t, uint64(200<<20), stats.MemoryLimit)
		assert.Equal(t, uint64(100<<20), stats.DiskTotal)
		assert.False(t, stats.SampledAt.IsZero())
	})

	t.Run("one whose run is not running, and one this node holds no run of, are not sampled", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1"))
		require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 0))

		_, err := f.node.Query(ctx, snippet("task-1"), taskKind.ActionStats, nil)
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		_, err = f.node.Query(ctx, snippet("task-9"), taskKind.ActionStats, nil)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("its stats are asked of its node as a kind's query is, and answered as a vm's are", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1", live))
		require.NoError(t, f.engine.SetStats(f.run(t, "task-1").ID, vm.Stats{CPUPercent: 25, MemoryUsed: 64 << 20}))

		resource, err := kind.Encode(snippet("task-1", live))
		require.NoError(t, err)

		binding := kind.BindNode[taskKind.Spec, taskKind.Status](taskKind.Descriptor(), f.node)

		answer, err := binding.Query(ctx, kind.Query{Kind: taskKind.Name, UUID: "task-1", Action: taskKind.ActionStats, Resource: resource})
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(answer, &fields))
		assert.ElementsMatch(t, []string{"cpu_percent", "memory_used", "memory_limit", "disk_used", "disk_total", "network_rx", "network_tx", "sampled_at"}, slices.Collect(maps.Keys(fields)))

		var stats vmKind.Stats
		require.NoError(t, json.Unmarshal(answer, &stats))
		assert.Equal(t, 25.0, stats.CPUPercent)
		assert.Equal(t, uint64(64<<20), stats.MemoryUsed)
	})

	t.Run("and nothing else is asked of one on its node", func(t *testing.T) {
		t.Parallel()

		f := onNode()
		f.created(t, snippet("task-1"))

		_, err := f.node.Query(ctx, snippet("task-1"), taskKind.ActionState, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestNode_Endpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	f := onNode()
	f.created(t, snippet("task-1", func(t *taskKind.Task) {
		live(t)
		t.Spec.Ports = []port.Port{3000, 8080}
	}))

	endpoint, err := f.node.Endpoint(ctx, "request-task-1-abcde", 0)
	require.NoError(t, err)
	assert.Equal(t, kind.Endpoint{Port: 3000, Address: "vmhost:20000"}, endpoint, "the lowest port, when none is named")

	endpoint, err = f.node.Endpoint(ctx, "request-task-1-abcde", 8080)
	require.NoError(t, err)
	assert.Equal(t, kind.Endpoint{Port: 8080, Address: "vmhost:20001"}, endpoint)

	_, err = f.node.Endpoint(ctx, "request-task-1-abcde", 9090)
	assert.ErrorIs(t, err, domain.ErrNotExists, "a port it does not expose")

	_, err = f.node.Endpoint(ctx, "nobody-abcde", 0)
	assert.ErrorIs(t, err, domain.ErrNotExists)

	require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 0))

	_, err = f.node.Endpoint(ctx, "request-task-1-abcde", 3000)
	assert.ErrorIs(t, err, kind.ErrUnreachable, "one that ended serves nothing")
}

func TestNode_Attach(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// a shell that says back what it is typed, upper-cased.
	echoing := memory.WithExec(func(_ context.Context, _ string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, _ io.Writer) int {
		if strings.Join(options.Command, " ") != "/bin/sh" || !options.TTY {
			return 127
		}

		typed, _ := io.ReadAll(io.LimitReader(stdin, 3))
		_, _ = io.WriteString(stdout, strings.ToUpper(string(typed)))

		return 0
	})

	for name, tt := range map[string]struct {
		owner  string
		asking string
		err    error
	}{
		"a snippet's is anybody's, signed in or not": {owner: task.GuestOwnerUUID},
		"and so to somebody signed in":               {owner: task.GuestOwnerUUID, asking: "somebody"},
		"one that says of no owner is anybody's":     {owner: ""},
		"somebody's own is theirs":                   {owner: "owner-uuid", asking: "owner-uuid"},
		"and not there for anybody else":             {owner: "owner-uuid", asking: "somebody-else", err: domain.ErrNotExists},
		"nor for nobody":                             {owner: "owner-uuid", err: domain.ErrNotExists},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := onNode(echoing)
			f.created(t, snippet("task-1", func(t *taskKind.Task) { t.Metadata.OwnerUUID = tt.owner }))

			session, err := f.node.Attach(ctx, taskKind.ActionAttach, "task-1", tt.asking)
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			defer session.Close()

			_, err = session.Stdin().Write([]byte("abc"))
			require.NoError(t, err)

			said, err := io.ReadAll(io.LimitReader(session.Stdout(), 3))
			require.NoError(t, err)
			assert.Equal(t, "ABC", string(said))
		})
	}

	t.Run("one that is not running cannot be opened now", func(t *testing.T) {
		t.Parallel()

		f := onNode(echoing)
		f.created(t, snippet("task-1"))
		require.NoError(t, f.engine.Exit(f.run(t, "task-1").ID, 0))

		_, err := f.node.Attach(ctx, taskKind.ActionAttach, "task-1", "")
		assert.ErrorIs(t, err, kind.ErrUnreachable)
	})

	t.Run("and one that is not here is not there", func(t *testing.T) {
		t.Parallel()

		_, err := onNode(echoing).node.Attach(ctx, taskKind.ActionAttach, "task-9", "")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestFitted(t *testing.T) {
	t.Parallel()

	t.Run("outputs that fit together are kept whole", func(t *testing.T) {
		t.Parallel()

		outputs := []string{"hello\n", "", "bye\n"}

		assert.Equal(t, outputs, fitted(outputs, 1<<10))
	})

	t.Run("otherwise the larger ones share what the smaller leave, each keeping its end", func(t *testing.T) {
		t.Parallel()

		small := "ok\n"
		large := strings.Repeat("a", 1000) + "the end\n"
		larger := strings.Repeat("b", 2000) + "the very end\n"

		cut := fitted([]string{large, small, larger}, 600)

		assert.Equal(t, small, cut[1])
		assert.True(t, strings.HasSuffix(cut[0], "the end\n"))
		assert.True(t, strings.HasSuffix(cut[2], "the very end\n"))

		total := 0
		for _, output := range cut {
			written, err := json.Marshal(output)
			require.NoError(t, err)

			total += len(written)
		}

		assert.LessOrEqual(t, total, 600, "as they are written out")
		assert.InDelta(t, len(cut[0]), len(cut[2]), 2, "an even share each")
	})

	t.Run("what is escaped as it is written out is counted as escaped", func(t *testing.T) {
		t.Parallel()

		cut := fitted([]string{strings.Repeat("<&>\"\n", 400), "é ✓"}, 300)

		total := 0
		for _, output := range cut {
			written, err := json.Marshal(output)
			require.NoError(t, err)

			total += len(written)
		}

		assert.LessOrEqual(t, total, 300)
		assert.Equal(t, "é ✓", cut[1])
	})

	t.Run("an output is cut at a whole character", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "✓✓", last("✓✓✓", 7))
		assert.Equal(t, "✓✓", tailWithin("✓✓✓", 8))
	})
}

// A failed create is what the code runner answers a snippet that never ran
// with, so its result carries the run it was to be.
func TestBinding(t *testing.T) {
	t.Parallel()

	f := onNode(memory.WithCapacity(1, 100<<20, 1<<30))

	binding := kind.BindNode[taskKind.Spec, taskKind.Status](taskKind.Descriptor(), f.node)
	assert.True(t, binding.Attaches())
	assert.True(t, binding.Exposes())

	resource, err := kind.Encode(snippet("task-1"))
	require.NoError(t, err)

	result := binding.Execute(context.Background(), kind.ActOnResource{ID: "command-1", Kind: taskKind.Name, UUID: "task-1", Action: taskKind.ActionCreate, Node: nodeName, Resource: resource})
	assert.False(t, result.OK)
	assert.NotEmpty(t, result.Reason)

	var status taskKind.Status
	require.NoError(t, json.Unmarshal(result.Status, &status))
	assert.Equal(t, taskKind.Failed, status.State)
	require.NotNil(t, status.Run)
	assert.Equal(t, "request-task-1", status.Run.Name)
}
