package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// travel sends a value the way a message does, and reads it back.
func travel[T any](t *testing.T, value T) T {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	var arrived T
	require.NoError(t, json.Unmarshal(payload, &arrived))

	return arrived
}

func TestSpec(t *testing.T) {
	t.Parallel()

	spec := vm.Spec{
		ID:             "vm-uuid",
		Kind:           vm.KindDocker,
		Image:          "docker:29-dind",
		Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:          []port.Port{80, 443},
		Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		PersistentDisk: true,
		Labels:         map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelOwner: "owner-uuid"},
		Command:        []string{"python", "-c", "print(1)"},
		Env:            []string{"A=1"},
		WorkingDir:     "/srv",
	}

	t.Run("a spec arrives as it left", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, spec, travel(t, NewSpec(spec)).ToVM())
	})

	t.Run("what travels is not the spec it was made from", func(t *testing.T) {
		t.Parallel()

		original := spec
		original.Labels = map[string]string{"a": "b"}
		original.Ports = []port.Port{80}

		travelling := NewSpec(original)
		travelling.Labels["a"] = "changed"
		travelling.Ports[0] = 8080

		assert.Equal(t, "b", original.Labels["a"])
		assert.Equal(t, port.Port(80), original.Ports[0])
	})

	t.Run("it travels under the names the rest of the workload uses", func(t *testing.T) {
		t.Parallel()

		payload, err := json.Marshal(VMScheduled{VMUUID: "vm-uuid", NodeName: "workload-orchestrator-01", Spec: NewSpec(spec)})
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))
		assert.Contains(t, fields, "vm_uuid")
		assert.Contains(t, fields, "node_name")
		assert.Contains(t, fields, "spec")
		assert.NotContains(t, fields, "snapshot_uuid", "a vm made from its image names no snapshot")

		var travelled map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(fields["spec"], &travelled))
		assert.Contains(t, travelled, "persistent_disk")
		assert.Contains(t, travelled, "working_dir")
	})
}

func TestInfo(t *testing.T) {
	t.Parallel()

	info := vm.Info{
		Engine:    "microsandbox",
		Version:   "0.7.6",
		CPUs:      8,
		Memory:    16 << 30,
		Disk:      200 << 30,
		Allocated: vm.Resources{CPUs: 3, Memory: 4 << 30, Disk: 40 << 30},
	}

	assert.Equal(t, info, travel(t, NewInfo(info)).ToVM())
}

func TestStats(t *testing.T) {
	t.Parallel()

	stats := vm.Stats{
		CPUPercent:  12.5,
		MemoryUsed:  1 << 30,
		MemoryLimit: 2 << 30,
		DiskUsed:    3 << 30,
		DiskTotal:   20 << 30,
		NetworkRx:   1024,
		NetworkTx:   2048,
		SampledAt:   time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, stats, travel(t, NewStats(stats)).ToVM())
}
