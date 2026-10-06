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

	t.Run("it lives in a vm, goes with it, and is what a restored one holds", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "stack", d.Name)
		assert.Equal(t, "stacks", d.Plural)
		assert.Equal(t, kind.OnNode, d.StateBy)
		assert.Equal(t, "vm", d.Parent)
		assert.Equal(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}, d.OnParent)
		assert.False(t, d.Endpoints, "a stack's ports are its vm's")
	})

	t.Run("its actions are asked under the permissions stacks always had", func(t *testing.T) {
		t.Parallel()

		permissions := map[string]string{}
		for _, a := range d.Actions {
			if !a.Internal {
				admin, _ := d.Permissions(a.Permission)
				permissions[a.Name] = admin
			}
		}

		assert.Equal(t, map[string]string{
			"start":   "workload.stacks.manage",
			"stop":    "workload.stacks.manage",
			"restart": "workload.stacks.manage",
			"delete":  "workload.stacks.delete",
			"state":   "workload.stacks.show",
		}, permissions)

		create, _ := d.Action(stack.ActionCreate)
		apply, _ := d.Action(stack.ActionApply)
		assert.True(t, create.Internal, "deploying is the workload's own to ask")
		assert.True(t, apply.Internal)
	})

	t.Run("what each action may be asked in", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			action string
			state  kind.State
			allows bool
		}{
			"made where it is not":                      {action: stack.ActionCreate, state: stack.Waiting, allows: true},
			"and not over one that is":                  {action: stack.ActionCreate, state: stack.Running},
			"applied again when it lost something":      {action: stack.ActionApply, state: stack.Degraded, allows: true},
			"or failed":                                 {action: stack.ActionApply, state: stack.Failed, allows: true},
			"started when stopped":                      {action: stack.ActionStart, state: stack.Stopped, allows: true},
			"or waiting, which makes it":                {action: stack.ActionStart, state: stack.Waiting, allows: true},
			"not while it deploys":                      {action: stack.ActionStart, state: stack.Deploying},
			"stopped when it runs":                      {action: stack.ActionStop, state: stack.Running, allows: true},
			"not when it is not in its vm":              {action: stack.ActionStop, state: stack.Waiting},
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

	t.Run("it is deleted with its volumes or without", func(t *testing.T) {
		t.Parallel()

		remove, _ := d.Action(stack.ActionDelete)

		value, invalid, err := remove.Payload.Decode([]byte(`{"remove_volumes":true}`))
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Equal(t, stack.DeletePayload{RemoveVolumes: true}, value)

		value, _, err = remove.Payload.Decode(nil)
		require.NoError(t, err)
		assert.Equal(t, stack.DeletePayload{}, value, "nothing keeps them")
	})
}

func TestMachine(t *testing.T) {
	t.Parallel()

	m := stack.Machine()

	assert.Empty(t, m.Validate())
	assert.Equal(t, stack.Waiting, m.Initial, "a stack is admitted waiting to be deployed")

	for name, tt := range map[string]struct {
		from  kind.State
		on    kind.Trigger
		to    kind.State
		taken bool
	}{
		"made, it deploys":                           {from: stack.Waiting, on: kind.OnAction(stack.ActionCreate), to: stack.Deploying, taken: true},
		"applied again, it deploys":                  {from: stack.Degraded, on: kind.OnAction(stack.ActionApply), to: stack.Deploying, taken: true},
		"a deploy ends running":                      {from: stack.Deploying, on: kind.OnObserved(stack.Running), to: stack.Running, taken: true},
		"or degraded":                                {from: stack.Deploying, on: kind.OnObserved(stack.Degraded), to: stack.Degraded, taken: true},
		"and nothing its vm has of it ends it early": {from: stack.Deploying, on: kind.OnObserved(kind.Missing), to: stack.Deploying},
		"nor its vm stopping under it":               {from: stack.Deploying, on: kind.OnObserved(stack.Waiting), to: stack.Deploying},
		"a stop ends stopped":                        {from: stack.Stopping, on: kind.OnObserved(stack.Stopped), to: stack.Stopped, taken: true},
		"and is not undone by a report sent before":  {from: stack.Stopping, on: kind.OnObserved(stack.Running), to: stack.Stopping},
		"a restart ends running":                     {from: stack.Restarting, on: kind.OnObserved(stack.Running), to: stack.Running, taken: true},
		"one being removed is gone only when it is":  {from: stack.Removing, on: kind.OnObserved(kind.Missing), to: stack.Removing},
		"at rest, it is what its node says":          {from: stack.Running, on: kind.OnObserved(stack.Degraded), to: stack.Degraded, taken: true},
		"waiting on its vm too":                      {from: stack.Running, on: kind.OnObserved(stack.Waiting), to: stack.Waiting, taken: true},
		"and back from waiting when its vm runs":     {from: stack.Waiting, on: kind.OnObserved(stack.Stopped), to: stack.Stopped, taken: true},
		"one its vm has nothing of waits to be made": {from: stack.Running, on: kind.OnObserved(kind.Missing), to: stack.Waiting, taken: true},
		"as does one waiting already":                {from: stack.Waiting, on: kind.OnObserved(kind.Missing), to: stack.Waiting, taken: true},
		"but a failed one stays failed":              {from: stack.Failed, on: kind.OnObserved(kind.Missing), to: stack.Failed},
		"a failure is believed from anywhere":        {from: stack.Restarting, on: kind.OnObserved(stack.Failed), to: stack.Failed, taken: true},
		"deleted from anywhere":                      {from: stack.Starting, on: kind.OnAction(stack.ActionDelete), to: stack.Removing, taken: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, taken := m.Next(tt.from, tt.on)

			assert.Equal(t, tt.to, to)
			assert.Equal(t, tt.taken, taken)
		})
	}
}

func TestInFlightOf(t *testing.T) {
	t.Parallel()

	for _, d := range stack.Descriptor().Actions {
		if d.Mode != kind.ModeCommand {
			continue
		}

		state := stack.InFlightOf(d.Name)

		assert.True(t, stack.Machine().IsInFlight(state), "%s is carried out in flight, as %q", d.Name, state)

		for _, from := range []kind.State{stack.Waiting, stack.Degraded, stack.Stopped, stack.Running, stack.Failed} {
			if to, moved := stack.Machine().Next(from, kind.OnAction(d.Name)); moved {
				assert.Equal(t, state, to, "%s from %s", d.Name, from)
			}
		}
	}

	assert.Empty(t, stack.InFlightOf(stack.ActionState))
}

func TestVMOf(t *testing.T) {
	t.Parallel()

	s := stack.Stack{
		Metadata: kind.Metadata{Owners: []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}},
		Spec:     stack.Spec{VM: stack.VMChoice{UUID: "vm-uuid"}},
	}

	assert.Equal(t, "vm-uuid", stack.VMOf(s))

	s.Metadata.Owners = nil
	assert.Equal(t, "vm-uuid", stack.VMOf(s), "the one it was admitted into")
}

func TestVMChoice(t *testing.T) {
	t.Parallel()

	chosen, err := json.Marshal(stack.VMChoice{UUID: "vm-uuid", New: &stack.NewVM{Ports: nil}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"uuid":"vm-uuid","new":{}}`, string(chosen), "one made with the defaults says so")

	none, err := json.Marshal(stack.NewVM{Ports: []port.Port{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ports":[]}`, string(none), "ports given empty are none, and travel as none")

	assert.True(t, stack.VMChoice{New: &stack.NewVM{}}.Created())
	assert.False(t, stack.VMChoice{UUID: "vm-uuid"}.Created())
}
