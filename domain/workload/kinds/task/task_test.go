package task

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := Descriptor()

	t.Run("it keeps every rule a kind keeps", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, kind.Check(d))
	})

	t.Run("its actions are asked under the vms' permissions, among which its runs are shown", func(t *testing.T) {
		t.Parallel()

		for verb, want := range map[string]string{
			"manage": "workload.vms.manage",
			"delete": "workload.vms.delete",
			"show":   "workload.vms.show",
			"logs":   "workload.vms.logs",
			"attach": "workload.vms.attach",
		} {
			admin, self := d.Permissions(verb)

			assert.Equal(t, want, admin)
			assert.Equal(t, "self."+want, self)
		}
	})

	t.Run("its terminal is anybody's to ask for, and its node says whom it opens for", func(t *testing.T) {
		t.Parallel()

		attach, found := d.Action(ActionAttach)
		require.True(t, found)
		assert.Equal(t, kind.ModeStream, attach.Mode)
		assert.True(t, attach.Public)
		assert.Equal(t, []kind.State{Running}, attach.AllowedIn)
	})

	t.Run("it is run only by the workload, and stopped, killed and deleted by whoever may", func(t *testing.T) {
		t.Parallel()

		create, _ := d.Action(ActionCreate)
		assert.True(t, create.Internal)

		for _, state := range []kind.State{Scheduled, Running, Restarting} {
			assert.True(t, d.Allows(ActionStop, state), state)
			assert.True(t, d.Allows(ActionKill, state), state)
		}

		for _, state := range []kind.State{Created, Stopping, Stopped, Completed, Failed, Deleting} {
			assert.False(t, d.Allows(ActionStop, state), state)
		}

		for _, state := range Machine().States {
			assert.Equal(t, state != Deleted, d.Allows(ActionDelete, state), state)
		}
	})

	t.Run("its ports are served through the ingress", func(t *testing.T) {
		t.Parallel()

		assert.True(t, d.Endpoints)
	})
}

func TestMachine(t *testing.T) {
	t.Parallel()

	m := Machine()

	for name, tt := range map[string]struct {
		from kind.State
		on   kind.Trigger
		to   kind.State
	}{
		"a task admitted is run":                             {from: Created, on: kind.OnAction(ActionCreate), to: Scheduled},
		"and one that failed, again":                         {from: Failed, on: kind.OnAction(ActionCreate), to: Scheduled},
		"and a service that stopped unasked, again":          {from: Stopped, on: kind.OnAction(ActionCreate), to: Scheduled},
		"its run comes up":                                   {from: Scheduled, on: kind.OnObserved(Running), to: Running},
		"or has ended by the time its node says anything":    {from: Scheduled, on: kind.OnObserved(Completed), to: Completed},
		"or could not be made":                               {from: Scheduled, on: kind.OnObserved(Failed), to: Failed},
		"a running one runs to its end":                      {from: Running, on: kind.OnObserved(Completed), to: Completed},
		"or fails":                                           {from: Running, on: kind.OnObserved(Failed), to: Failed},
		"or is gone from its node, which is a failure":       {from: Running, on: kind.OnObserved(kind.Missing), to: Failed},
		"a running one is stopped":                           {from: Running, on: kind.OnAction(ActionStop), to: Stopping},
		"or killed, which is a stop":                         {from: Running, on: kind.OnAction(ActionKill), to: Stopping},
		"and ends":                                           {from: Stopping, on: kind.OnObserved(Completed), to: Completed},
		"or is stopped with nothing left of it on its node":  {from: Stopping, on: kind.OnObserved(kind.Missing), to: Stopped},
		"anything is deleted":                                {from: Completed, on: kind.OnAction(ActionDelete), to: Deleting},
		"and is gone once its node holds nothing of it":      {from: Deleting, on: kind.OnObserved(kind.Missing), to: Deleted},
		"one at rest is what its node says":                  {from: Failed, on: kind.OnObserved(Running), to: Running},
		"one stopping is not moved by a beat from before":    {from: Stopping, on: kind.OnObserved(Running), to: Stopping},
		"nor one being run by a node that has not run it":    {from: Scheduled, on: kind.OnObserved(kind.Missing), to: Scheduled},
		"one that has ended is not missing, it is ended":     {from: Completed, on: kind.OnObserved(kind.Missing), to: Completed},
		"one admitted and not run yet is missing everywhere": {from: Created, on: kind.OnObserved(kind.Missing), to: Created},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, _ := m.Next(tt.from, tt.on)
			assert.Equal(t, tt.to, to)
		})
	}

	t.Run("a stopped, completed or failed task has ended, and nothing else has", func(t *testing.T) {
		t.Parallel()

		for _, state := range m.States {
			assert.Equal(t, state == Stopped || state == Completed || state == Failed, Ended(state), state)
		}
	})
}

func TestStateOf(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		status   task.Status
		kind     task.Kind
		exitCode int
		want     kind.State
	}{
		"made and not started":                      {status: task.StatusCreated, kind: task.KindJob, want: Scheduled},
		"running":                                   {status: task.StatusRunning, kind: task.KindJob, want: Running},
		"restarting":                                {status: task.StatusRestarting, kind: task.KindService, want: Restarting},
		"paused":                                    {status: task.StatusPaused, kind: task.KindService, want: Stopped},
		"a job that exited with 0 completed":        {status: task.StatusExited, kind: task.KindJob, want: Completed},
		"one that chose another code failed":        {status: task.StatusExited, kind: task.KindJob, exitCode: 124, want: Failed},
		"one killed by a signal was cut short":      {status: task.StatusExited, kind: task.KindJob, exitCode: 137, want: Completed},
		"a service that exited stopped, either way": {status: task.StatusExited, kind: task.KindService, exitCode: 1, want: Stopped},
		"a dead one failed":                         {status: task.StatusDead, kind: task.KindJob, want: Failed},
		"and so did one nothing knows":              {status: task.Status(42), kind: task.KindJob, want: Failed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, StateOf(tt.status, tt.kind, tt.exitCode))
		})
	}
}

func TestSpec(t *testing.T) {
	t.Parallel()

	t.Run("one that names nothing is an isolated job, worth no retries", func(t *testing.T) {
		t.Parallel()

		var s Spec

		assert.Equal(t, task.KindJob, s.TaskKind())
		assert.Equal(t, network.PolicyIsolated, s.Policy())
		assert.Equal(t, 0, s.Retries())
	})

	t.Run("a service is worth what a service is", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, task.DefaultMaxRetries(task.KindService), Spec{Kind: task.KindService}.Retries())
	})

	t.Run("and one that says is worth what it says", func(t *testing.T) {
		t.Parallel()

		forever := task.RetryForever

		assert.Equal(t, task.RetryForever, Spec{Kind: task.KindService, MaxRetries: &forever}.Retries())
	})

	t.Run("its limits are its run's, cores and bytes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, task.ResourceLimits{Cpu: 1.5, Memory: 512 << 20, Disk: 100 << 20}, Limits{CPU: 1.5, Memory: 512 << 20, Disk: 100 << 20}.ResourceLimits())
	})
}

// A status travels as JSON, beside every other kind's, and what the control
// plane alone counts is never something a node says.
func TestStatus_JSON(t *testing.T) {
	t.Parallel()

	t.Run("a node's says nothing of the retries", func(t *testing.T) {
		t.Parallel()

		payload, err := json.Marshal(Status{Status: kind.Status{State: Running}, Run: &Run{Name: "request", Kind: task.KindJob}})
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))

		assert.NotContains(t, fields, "retries")
		assert.Contains(t, fields, "run")
	})

	t.Run("a run is a job's unless it says it is a service's", func(t *testing.T) {
		t.Parallel()

		var nothing *Run

		assert.False(t, nothing.Job())
		assert.True(t, (&Run{}).Job())
		assert.True(t, (&Run{Kind: task.KindJob}).Job())
		assert.False(t, (&Run{Kind: task.KindService}).Job())
	})
}
