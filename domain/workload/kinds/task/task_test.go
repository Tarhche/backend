package task

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
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

	t.Run("what its run uses is sampled by its node while it runs, for whoever may see a vm, as a vm's is", func(t *testing.T) {
		t.Parallel()

		stats, found := d.Action(ActionStats)
		require.True(t, found)

		sampled, _ := vmKind.Descriptor().Action(vmKind.ActionStats)
		assert.Equal(t, sampled.Name, stats.Name)
		assert.Equal(t, kind.OnNode, stats.Runs)
		assert.Equal(t, kind.ModeQuery, stats.Mode)
		assert.Equal(t, []kind.State{Running}, stats.AllowedIn)
		assert.Equal(t, sampled.Payload.Type(), stats.Payload.Type(), "it is asked nothing")

		admin, self := d.Permissions(stats.Permission)
		assert.Equal(t, "workload.vms.show", admin)
		assert.Equal(t, "self.workload.vms.show", self)
	})

	t.Run("it is changed while it runs, by whoever may change a vm, and given what it is changed to by the workload alone", func(t *testing.T) {
		t.Parallel()

		update, found := d.Action(ActionUpdate)
		require.True(t, found)
		assert.Equal(t, kind.OnControlPlane, update.Runs)
		assert.Equal(t, kind.ModeCommand, update.Mode)
		assert.Equal(t, []kind.State{Running}, update.AllowedIn)

		admin, self := d.Permissions(update.Permission)
		assert.Equal(t, "workload.vms.update", admin)
		assert.Equal(t, "self.workload.vms.update", self)

		reconfigure, found := d.Action(ActionReconfigure)
		require.True(t, found)
		assert.Equal(t, kind.OnNode, reconfigure.Runs)
		assert.True(t, reconfigure.Internal)
		assert.Equal(t, []kind.State{Running}, reconfigure.AllowedIn)
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
		"a running one is reconfigured, which restarts it":   {from: Running, on: kind.OnAction(ActionReconfigure), to: Restarting},
		"and runs again":                                     {from: Restarting, on: kind.OnObserved(Running), to: Running},
		"or has ended by the time it is answered":            {from: Restarting, on: kind.OnObserved(Completed), to: Completed},
		"or is stopped while it restarts":                    {from: Restarting, on: kind.OnAction(ActionStop), to: Stopping},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, _ := m.Next(tt.from, tt.on)
			assert.Equal(t, tt.to, to)
		})
	}

	t.Run("a restart is waited on until it is answered, whatever its node says meanwhile", func(t *testing.T) {
		t.Parallel()

		assert.True(t, m.IsInFlight(Restarting))
		assert.True(t, m.IsAnswered(Restarting))
	})

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

	t.Run("its config is what its node gives its run, a policy that names nothing the default one", func(t *testing.T) {
		t.Parallel()

		s := Spec{Ports: []port.Port{3000}, Limits: Limits{CPU: 2, Memory: 200 << 20, Disk: 100 << 20}}

		assert.Equal(t, Config{Ports: []port.Port{3000}, NetworkPolicy: network.PolicyIsolated, Limits: s.Limits}, s.Config())
		assert.True(t, s.Config().Equal(Config{Ports: []port.Port{3000}, Limits: s.Limits}))
		assert.True(t, Spec{}.Config().Equal(Config{Ports: []port.Port{}, NetworkPolicy: network.PolicyIsolated}), "no ports are none")

		for name, other := range map[string]Config{
			"another port":   {Ports: []port.Port{3001}, Limits: s.Limits},
			"another policy": {Ports: []port.Port{3000}, NetworkPolicy: network.PolicyPublic, Limits: s.Limits},
			"other limits":   {Ports: []port.Port{3000}, Limits: Limits{CPU: 2, Memory: 300 << 20, Disk: 100 << 20}},
		} {
			assert.False(t, s.Config().Equal(other), name)
		}
	})
}

func TestLimits_Validate(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		limits Limits
		want   domain.ValidationErrors
	}{
		"what a snippet is given":     {limits: Limits{CPU: 2, Memory: 200 << 20, Disk: 100 << 20}, want: domain.ValidationErrors{}},
		"nothing at all":              {limits: Limits{}, want: domain.ValidationErrors{"limits.cpu": "required_field", "limits.memory": "required_field", "limits.disk": "required_field"}},
		"less memory than a vm takes": {limits: Limits{CPU: 1, Memory: task.MinMemory - 1, Disk: 1}, want: domain.ValidationErrors{"limits.memory": "memory_below_minimum"}},
		"more than a record keeps":    {limits: Limits{CPU: 1, Memory: kind.MaxInteger + 1, Disk: kind.MaxInteger + 1}, want: domain.ValidationErrors{"limits.memory": "too_large", "limits.disk": "too_large"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.limits.Validate())
		})
	}
}

func TestUpdatePayload(t *testing.T) {
	t.Parallel()

	name := func(n string) *string { return &n }
	ports := func(p ...port.Port) *[]port.Port { return &p }
	policy := func(p network.Policy) *network.Policy { return &p }

	for title, tt := range map[string]struct {
		payload UpdatePayload
		want    domain.ValidationErrors
	}{
		"nothing changes nothing":    {payload: UpdatePayload{}, want: domain.ValidationErrors{}},
		"a name, ports and a policy": {payload: UpdatePayload{Name: name("a run"), Ports: ports(3000, 8080), NetworkPolicy: policy(network.PolicyOutbound)}, want: domain.ValidationErrors{}},
		"no name":                    {payload: UpdatePayload{Name: name("  ")}, want: domain.ValidationErrors{"name": "required_field"}},
		"a name no listing shows":    {payload: UpdatePayload{Name: name(strings.Repeat("a", MaxNameLength+1))}, want: domain.ValidationErrors{"name": "invalid_name"}},
		"port zero":                  {payload: UpdatePayload{Ports: ports(0)}, want: domain.ValidationErrors{"ports": "invalid_value"}},
		"a port past the last":       {payload: UpdatePayload{Ports: ports(65536)}, want: domain.ValidationErrors{"ports": "invalid_value"}},
		"a policy nobody knows":      {payload: UpdatePayload{NetworkPolicy: policy("host")}, want: domain.ValidationErrors{"network_policy": "invalid_network_policy"}},
		"limits held to theirs":      {payload: UpdatePayload{Limits: &Limits{CPU: 1, Memory: task.MinMemory - 1, Disk: 1}}, want: domain.ValidationErrors{"limits.memory": "memory_below_minimum"}},
	} {
		t.Run(title, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.payload.Validate())
		})
	}

	t.Run("its name is its record's, and the rest its run's", func(t *testing.T) {
		t.Parallel()

		assert.False(t, UpdatePayload{Name: name("renamed")}.Changes())
		assert.True(t, UpdatePayload{Ports: ports()}.Changes())
		assert.True(t, UpdatePayload{NetworkPolicy: policy(network.PolicyPublic)}.Changes())
		assert.True(t, UpdatePayload{Limits: &Limits{}}.Changes())
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
		assert.NotContains(t, fields, "applied", "nor of a config it was given, unless it gave one")
		assert.Contains(t, fields, "run")
	})

	t.Run("what its run uses is written as a vm's sample is, and nothing as nothing, which is written over what it said before", func(t *testing.T) {
		t.Parallel()

		sample := vmKind.StatsOf(vm.Stats{CPUPercent: 12.5, MemoryUsed: 1, MemoryLimit: 2, DiskUsed: 3, DiskTotal: 4, NetworkRx: 5, NetworkTx: 6, SampledAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)})

		fieldsOf := func(t *testing.T, status any) map[string]json.RawMessage {
			t.Helper()

			payload, err := json.Marshal(status)
			require.NoError(t, err)

			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(payload, &fields))

			return fields
		}

		sampled := fieldsOf(t, Status{Status: kind.Status{State: Running}, Run: &Run{Name: "request", Kind: task.KindJob}, Stats: sample})
		asAVM := fieldsOf(t, vmKind.Status{Status: kind.Status{State: vmKind.Running}, Stats: sample})
		assert.JSONEq(t, string(asAVM["stats"]), string(sampled["stats"]), "in a vm's words")

		ended := fieldsOf(t, Status{Status: kind.Status{State: Completed}, Run: &Run{Name: "request", Kind: task.KindJob}})
		require.Contains(t, ended, "stats", "said, rather than left out, so that a record keeping what it said before has it written over")
		assert.JSONEq(t, `null`, string(ended["stats"]))

		var read Status
		require.NoError(t, json.Unmarshal([]byte(`{"state":"running","stats":`+string(sampled["stats"])+`}`), &read))
		assert.Equal(t, sample, read.Stats)
	})

	t.Run("a run is a job's only when it says so", func(t *testing.T) {
		t.Parallel()

		var nothing *Run

		assert.False(t, nothing.Job())
		assert.False(t, (&Run{}).Job())
		assert.True(t, (&Run{Kind: task.KindJob}).Job())
		assert.False(t, (&Run{Kind: task.KindService}).Job())
	})
}
