package requirements_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

func TestWant(t *testing.T) {
	t.Parallel()

	fan := kindstest.Spec{Blades: 3}

	got, err := requirements.Want[kindstest.Spec](kindstest.Kind, fan)
	require.NoError(t, err)
	assert.Equal(t, fan, got, "a want handed over as one")

	got, err = requirements.Want[kindstest.Spec](kindstest.Kind, &fan)
	require.NoError(t, err)
	assert.Equal(t, fan, got, "or as a pointer to one")

	for name, spec := range map[string]any{
		"nothing":                  nil,
		"a pointer to nothing":     (*kindstest.Spec)(nil),
		"another kind's":           kindstest.Status{},
		"what it would be as json": json.RawMessage(`{"blades": 3}`),
	} {
		_, err := requirements.Want[kindstest.Spec](kindstest.Kind, spec)
		assert.ErrorIs(t, err, kind.ErrInvalidPayload, name)
	}
}

func TestGoing(t *testing.T) {
	t.Parallel()

	assert.True(t, requirements.Going(kind.Status{State: kindstest.Running, Expected: kind.Deleted}), "expected deleted, it is on its way out")
	assert.True(t, requirements.Going(kind.Status{State: kind.Deleted}), "and so is one deleted")
	assert.False(t, requirements.Going(kind.Status{State: kind.Failed, Expected: kindstest.Running}), "a failed one is not")
}

func TestRecords_Wait(t *testing.T) {
	t.Parallel()

	d := kindstest.Descriptor()
	for i := range d.Actions {
		if d.Actions[i].Name == "create" {
			d.Actions[i].Timeout = kind.TimeoutPull
		}
	}

	records := requirements.New(requirements.Dependencies{
		Timeouts: map[kind.Timeout]time.Duration{kind.TimeoutPull: 10 * time.Minute},
		Patience: 5 * time.Minute,
	})

	assert.Equal(t, 15*time.Minute, records.Wait(d, "create"), "what its node may take over it, and the patience beyond")
	assert.Equal(t, 5*time.Minute, records.Wait(d, "stop"), "and one given no timeout, the patience alone")
}

func TestRefOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, graph.Ref{Kind: kindstest.Kind, UUID: "fan-uuid"}, requirements.RefOf(kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)))
}

func TestRefused(t *testing.T) {
	t.Parallel()

	err := requirements.Refused("vm", domain.ValidationErrors{"vm.resources.memory": "too_small", "vm": "no_capacity"})

	assert.EqualError(t, err, "the vm was refused: vm: no_capacity, vm.resources.memory: too_small", "field by field, in order")
}

func TestInadmissible(t *testing.T) {
	t.Parallel()

	err := requirements.Inadmissible("vm", domain.ValidationErrors{"vm.resources.memory": "too_small", "vm": "no_capacity"})

	var refused *graph.Refused
	require.ErrorAs(t, fmt.Errorf("vm/v-uuid: %w", err), &refused, "a refusal of the graph's, however it is wrapped")
	assert.Equal(t, "no_capacity", refused.Code, "the code of its first field")
	assert.Equal(t, "the vm was refused: vm: no_capacity, vm.resources.memory: too_small", refused.Message, "and every field said")
}

func TestNotDone(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		record resource.Record
		said   string
	}{
		"what it is, and why it is": {
			record: kindstest.AFan("fan-uuid", kind.Failed, kindstest.Running, func(f *kindstest.Fan) { f.Status.Reason = "the blades are stuck" }),
			said:   "the fan kitchen is not made: it is failed: the blades are stuck",
		},
		"or what its last command failed with, when it says no reason": {
			record: func() resource.Record {
				r := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)
				r.Status = json.RawMessage(`{"state": "running", "failure": {"code": "invalid", "message": "the room is too small"}}`)

				return r
			}(),
			said: "the fan kitchen is not made: it is running: the room is too small",
		},
		"or what it is alone": {
			record: kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running),
			said:   "the fan kitchen is not made: it is starting",
		},
		"by its uuid, when it is called nothing": {
			record: kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running, func(f *kindstest.Fan) { f.Metadata.Name = "" }),
			said:   "the fan fan-uuid is not made: it is starting",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.EqualError(t, requirements.NotDone(tt.record, "made"), tt.said)
		})
	}

	assert.EqualError(t, requirements.Gone(kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)), "the fan kitchen is gone")
}
