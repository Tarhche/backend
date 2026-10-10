package stack_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

func TestWant(t *testing.T) {
	t.Parallel()

	w := stack.Want{UUID: "s-uuid", OwnerUUID: "owner", Slug: "shop-abcde", VM: stack.VMChoice{UUID: "vm-uuid"}}

	t.Run("it is required by its record's uuid", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, graph.Requirement{Kind: stack.Name, Name: "s-uuid", Spec: w}, w.Requirement())
		assert.Equal(t, graph.NodeID("stack/s-uuid"), w.Requirement().ID())
	})

	t.Run("what it makes is named under its slug", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "shop-abcde_web", w.Named("web"))
		assert.Equal(t, "shop-abcde_default", w.Named(stack.DefaultNetwork))
	})
}

func TestStateOf(t *testing.T) {
	t.Parallel()

	services := func(states ...stack.ServiceState) []stack.ServiceStatus {
		of := make([]stack.ServiceStatus, len(states))
		for i, state := range states {
			of[i] = stack.ServiceStatus{Name: string(rune('a' + i)), State: state}
		}

		return of
	}

	for name, tt := range map[string]struct {
		services []stack.ServiceStatus
		state    kind.State
	}{
		"each runs":                                    {services: services(stack.ServiceRunning, stack.ServiceRunning), state: stack.Running},
		"one runs and a one-off ran to its end":        {services: services(stack.ServiceRunning, stack.ServiceCompleted), state: stack.Running},
		"one runs and one stopped":                     {services: services(stack.ServiceRunning, stack.ServiceStopped), state: stack.Degraded},
		"one runs and one has no container":            {services: services(stack.ServiceRunning, stack.ServiceMissing), state: stack.Degraded},
		"none runs":                                    {services: services(stack.ServiceStopped, stack.ServiceStopped), state: stack.Stopped},
		"none runs, a one-off among them that is done": {services: services(stack.ServiceCompleted, stack.ServiceStopped), state: stack.Stopped},
		"each is a one-off that is done":               {services: services(stack.ServiceCompleted), state: stack.Stopped},
		"none has a container":                         {services: services(stack.ServiceMissing), state: stack.Stopped},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.state, stack.StateOf(tt.services))
		})
	}
}
