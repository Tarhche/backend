package stack_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := stack.Descriptor()

	t.Run("it keeps every rule a kind keeps", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, kind.Check(d))
	})

	t.Run("it lives in a vm, its state is the control plane's, and outlives a restore of its vm's disk", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "stack", d.Name)
		assert.Equal(t, "stacks", d.Plural)
		assert.Equal(t, kind.OnControlPlane, d.StateBy, "no node says what a stack is doing")
		assert.Equal(t, "vm", d.Parent)
		assert.Equal(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep}, d.OnParent)
		assert.False(t, d.Endpoints, "a stack's ports are its vm's")
	})

	t.Run("each of its actions is the control plane's, and nothing is asked of a node", func(t *testing.T) {
		t.Parallel()

		for _, a := range d.Actions {
			assert.Equal(t, kind.OnControlPlane, a.Runs, "%s", a.Name)
			assert.False(t, a.Internal, "%s", a.Name)
			assert.Empty(t, a.Timeout, "%s", a.Name)
		}

		_, deploys := d.Action(stack.ActionCreate)
		assert.False(t, deploys, "nothing deploys a stack but its graph")

		_, applies := d.Action(stack.ActionApply)
		assert.False(t, applies)
	})

	t.Run("its actions are asked under the permissions stacks always had", func(t *testing.T) {
		t.Parallel()

		permissions := map[string]string{}
		for _, a := range d.Actions {
			admin, _ := d.Permissions(a.Permission)
			permissions[a.Name] = admin
		}

		assert.Equal(t, map[string]string{
			"start":   "workload.stacks.manage",
			"stop":    "workload.stacks.manage",
			"restart": "workload.stacks.manage",
			"delete":  "workload.stacks.delete",
			"state":   "workload.stacks.show",
		}, permissions)
	})

	t.Run("what each action may be asked in", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			action string
			state  kind.State
			allows bool
		}{
			"started when stopped":                      {action: stack.ActionStart, state: stack.Stopped, allows: true},
			"or degraded":                               {action: stack.ActionStart, state: stack.Degraded, allows: true},
			"or waiting":                                {action: stack.ActionStart, state: stack.Waiting, allows: true},
			"not while it deploys":                      {action: stack.ActionStart, state: stack.Deploying},
			"nor while it starts":                       {action: stack.ActionStart, state: stack.Starting},
			"stopped when it runs":                      {action: stack.ActionStop, state: stack.Running, allows: true},
			"or failed":                                 {action: stack.ActionStop, state: stack.Failed, allows: true},
			"not while it deploys, which is busy":       {action: stack.ActionStop, state: stack.Deploying},
			"nor while it waits on its vm":              {action: stack.ActionStop, state: stack.Waiting},
			"restarted when stopped":                    {action: stack.ActionRestart, state: stack.Stopped, allows: true},
			"not while it is being stopped":             {action: stack.ActionRestart, state: stack.Stopping},
			"deleted whatever it is doing":              {action: stack.ActionDelete, state: stack.Deploying, allows: true},
			"even while it is being deleted":            {action: stack.ActionDelete, state: stack.Removing, allows: true},
			"and asked what it is doing whatever it is": {action: stack.ActionState, state: stack.Restarting, allows: true},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, tt.allows, d.Allows(tt.action, tt.state))
			})
		}
	})

	t.Run("it is deleted with nothing, which keeps its volumes", func(t *testing.T) {
		t.Parallel()

		remove, _ := d.Action(stack.ActionDelete)

		assert.Equal(t, kind.NoPayload, remove.Payload)
		assert.Equal(t, stack.Deleted, remove.Desires)
	})
}

func TestMachine(t *testing.T) {
	t.Parallel()

	m := stack.Machine()

	assert.Empty(t, m.Validate())
	assert.Equal(t, stack.Deploying, m.Initial, "a stack is admitted to be made by its graph")
	assert.ElementsMatch(t, []kind.State{stack.Deploying, stack.Starting, stack.Stopping, stack.Restarting, stack.Removing}, m.InFlight)
	assert.Empty(t, m.Answered, "nothing is asked of a node, whose answer would say what came of it")

	for name, tt := range map[string]struct {
		from  kind.State
		on    kind.Trigger
		to    kind.State
		taken bool
	}{
		"made, it is what its containers make it":           {from: stack.Deploying, on: kind.OnObserved(stack.Running), to: stack.Running, taken: true},
		"degraded among it":                                 {from: stack.Deploying, on: kind.OnObserved(stack.Degraded), to: stack.Degraded, taken: true},
		"it waits while it deploys on what it needs":        {from: stack.Deploying, on: kind.OnObserved(stack.Waiting), to: stack.Waiting, taken: true},
		"and fails from anywhere":                           {from: stack.Restarting, on: kind.OnObserved(stack.Failed), to: stack.Failed, taken: true},
		"started, it starts":                                {from: stack.Stopped, on: kind.OnAction(stack.ActionStart), to: stack.Starting, taken: true},
		"and runs once its containers do":                   {from: stack.Starting, on: kind.OnObserved(stack.Running), to: stack.Running, taken: true},
		"but goes on starting while its vm is not up":       {from: stack.Starting, on: kind.OnObserved(stack.Waiting), to: stack.Starting},
		"stopped, it stops":                                 {from: stack.Running, on: kind.OnAction(stack.ActionStop), to: stack.Stopping, taken: true},
		"and is stopped once its containers are":            {from: stack.Stopping, on: kind.OnObserved(stack.Stopped), to: stack.Stopped, taken: true},
		"and goes on stopping while its vm is not up":       {from: stack.Stopping, on: kind.OnObserved(stack.Waiting), to: stack.Stopping},
		"restarted, it restarts":                            {from: stack.Degraded, on: kind.OnAction(stack.ActionRestart), to: stack.Restarting, taken: true},
		"and starts once its containers stopped":            {from: stack.Restarting, on: kind.OnObserved(stack.Starting), to: stack.Starting, taken: true},
		"but is not taken for running before it starts":     {from: stack.Restarting, on: kind.OnObserved(stack.Running), to: stack.Restarting},
		"at rest, it is what its graph says":                {from: stack.Running, on: kind.OnObserved(stack.Degraded), to: stack.Degraded, taken: true},
		"waiting on what it needs among it":                 {from: stack.Running, on: kind.OnObserved(stack.Waiting), to: stack.Waiting, taken: true},
		"and back from waiting once its graph made it":      {from: stack.Waiting, on: kind.OnObserved(stack.Stopped), to: stack.Stopped, taken: true},
		"deleted from anywhere":                             {from: stack.Starting, on: kind.OnAction(stack.ActionDelete), to: stack.Removing, taken: true},
		"and taken for nothing else while it is removed":    {from: stack.Removing, on: kind.OnObserved(stack.Running), to: stack.Removing},
		"nothing its vm has of it says anything of a stack": {from: stack.Running, on: kind.OnObserved(kind.Missing), to: stack.Running},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, taken := m.Next(tt.from, tt.on)

			assert.Equal(t, tt.to, to)
			assert.Equal(t, tt.taken, taken)
		})
	}
}

func TestVMOf(t *testing.T) {
	t.Parallel()

	s := stack.Stack{
		Metadata: kind.Metadata{Refs: []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}},
		Spec:     stack.Spec{VM: stack.VMChoice{UUID: "admitted-uuid"}},
	}

	assert.Equal(t, "vm-uuid", stack.VMOf(s), "the one it lives in, among its refs")

	s.Metadata.Owners, s.Metadata.Refs = s.Metadata.Refs, nil
	assert.Equal(t, "vm-uuid", stack.VMOf(s), "or among its owners, kept before refs")

	s.Metadata.Owners = nil
	assert.Equal(t, "admitted-uuid", stack.VMOf(s), "or the one it was admitted into")
}

func TestVMChoice(t *testing.T) {
	t.Parallel()

	kept, err := json.Marshal(stack.VMChoice{UUID: "vm-uuid"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"uuid":"vm-uuid"}`, string(kept), "one named, or kept, is its uuid and nothing else")

	var asked stack.VMChoice
	require.NoError(t, json.Unmarshal([]byte(`{"name":"builds","resources":{"cpus":2,"memory":1,"disk":2},"ports":[80],"network":{"ingress":"deny","egress":"allow"}}`), &asked))
	assert.Equal(t, stack.VMChoice{
		Name:      "builds",
		Resources: &stack.Resources{CPUs: 2, Memory: 1, Disk: 2},
		Ports:     []port.Port{80},
		Network:   &stack.Network{Ingress: "deny", Egress: "allow"},
	}, asked, "one to make is described where its uuid would be")

	none, err := json.Marshal(stack.VMChoice{Ports: []port.Port{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ports":[]}`, string(none), "ports given empty are none, and travel as none")

	assert.True(t, asked.Describes())
	assert.True(t, stack.VMChoice{Ports: []port.Port{}}.Describes(), "no ports at all is something to make one with")
	assert.False(t, stack.VMChoice{UUID: "vm-uuid"}.Describes())
	assert.False(t, stack.VMChoice{}.Describes(), "nothing is the defaults alone")
}
