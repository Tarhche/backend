package wire

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// travel is v after it has crossed the socket as JSON.
func travel[T any](t *testing.T, v T) T {
	t.Helper()

	encoded, err := json.Marshal(v)
	require.NoError(t, err)

	var arrived T
	require.NoError(t, json.Unmarshal(encoded, &arrived))

	return arrived
}

func TestSpec(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		spec vm.Spec
	}{
		{
			name: "a vm, everything said",
			spec: vm.Spec{
				ID:             "vm-1",
				Image:          "docker:29-dind",
				Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 1<<62 + 1},
				Ports:          []port.Port{80, 443, 8080},
				Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
				PersistentDisk: true,
				Labels:         map[string]string{vm.LabelOwner: "owner", vm.LabelPurpose: vm.PurposeVM},
				Env:            []string{"A=1"},
				WorkingDir:     "/srv",
			},
		},
		{
			name: "a code runner's task, keeping its image's entrypoint",
			spec: vm.Spec{
				ID:         "run-1",
				Image:      "python:3.12-alpine",
				Entrypoint: nil,
				Command:    []string{"python", "-c", "print(1)"},
			},
		},
		{
			name: "an entrypoint cleared is not one kept",
			spec: vm.Spec{
				ID:         "run-2",
				Entrypoint: []string{},
				Command:    []string{},
				Ports:      []port.Port{},
				Labels:     map[string]string{},
				Env:        []string{},
			},
		},
		{
			name: "nothing said",
			spec: vm.Spec{},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			arrived := travel(t, NewSpec(tc.spec)).ToVM()

			assert.Equal(t, tc.spec, arrived)
			assert.Equal(t, tc.spec.Entrypoint == nil, arrived.Entrypoint == nil, "nil stays nil and empty stays empty")
			assert.Equal(t, tc.spec.Command == nil, arrived.Command == nil)
			assert.Equal(t, tc.spec.HasMainProcess(), arrived.HasMainProcess())
		})
	}
}

func TestInstance(t *testing.T) {
	t.Parallel()

	instances := []vm.Instance{
		{
			ID:     "vm-1",
			State:  vm.InstanceRunning,
			Labels: map[string]string{vm.LabelVM: "vm-1"},
			Endpoints: []vm.Endpoint{
				{Port: 80, Address: "10.89.1.10:20000"},
				{Port: 443, Address: "10.89.1.10:20001"},
			},
			StartedAt: time.Date(2026, 10, 5, 12, 30, 1, 123456789, time.UTC),
		},
		{
			ID:       "run-1",
			State:    vm.InstanceExited,
			ExitCode: -1,
			Reason:   "killed",
		},
	}

	assert.Equal(t, instances, InstancesToVM(travel(t, NewInstances(instances))))
	assert.Nil(t, InstancesToVM(travel(t, NewInstances(nil))), "no listing stays no listing")
	assert.Equal(t, []vm.Instance{}, InstancesToVM(travel(t, NewInstances([]vm.Instance{}))))
}

func TestInfoStatsLogsArchive(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 5, 12, 0, 0, 1, time.UTC)

	info := vm.Info{
		Engine: "microsandbox", Version: "0.7.6",
		CPUs: 8, Memory: 1<<64 - 1, Disk: 200 << 30,
		Allocated: vm.Resources{CPUs: 3, Memory: 3 << 30, Disk: 40 << 30},
	}
	assert.Equal(t, info, travel(t, NewInfo(info)).ToVM(), "a budget past what a float holds is kept to the byte")

	stats := vm.Stats{
		CPUPercent: 37.123456789, MemoryUsed: 1 << 30, MemoryLimit: 2 << 30,
		DiskUsed: 3, DiskTotal: 4, NetworkRx: 5, NetworkTx: 6, SampledAt: at,
	}
	assert.Equal(t, stats, travel(t, NewStats(stats)).ToVM())

	lines := []vm.LogLine{{At: at, Source: vm.LogSourceMain, Line: "1"}, {At: at, Source: vm.LogSourceExec, Line: ""}}
	assert.Equal(t, lines, LogLinesToVM(travel(t, NewLogLines(lines))))

	archive := vm.Archive{Engine: "microsandbox/0.7.6", Kind: vm.KindDocker, Image: "docker:29-dind", Disk: 20 << 30, Size: 172_000_000}
	assert.Equal(t, archive, travel(t, NewArchive(archive)).ToVM())

	options := vm.ExecOptions{Command: []string{"/bin/sh"}, TTY: true, Rows: 40, Cols: 120, Env: []string{"TERM=xterm"}, WorkingDir: "/"}
	assert.Equal(t, options, travel(t, NewExecOptions(options)).ToVM())
}

func TestHeader(t *testing.T) {
	t.Parallel()

	spec := NewSpec(vm.Spec{ID: "vm-1", Labels: map[string]string{"k": "a value, with: what a header\r\ncannot carry"}})

	encoded, err := EncodeHeader(spec)
	require.NoError(t, err)
	assert.NotContains(t, encoded, "\n")

	var decoded Spec
	require.NoError(t, DecodeHeader(encoded, &decoded))
	assert.Equal(t, spec, decoded)

	assert.ErrorIs(t, DecodeHeader("not base64!", &decoded), ErrInvalid)
	assert.ErrorIs(t, DecodeHeader("bm90IGpzb24=", &decoded), ErrInvalid)
}

func TestPathVM(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/v1/vms/vm-1", PathVM("vm-1"))
	assert.Equal(t, "/v1/vms/vm-1/exec", PathVM("vm-1", ActionExec))
	assert.Equal(t, "/v1/vms/a%2Fb", PathVM("a/b"), "an id is one segment of the path, whatever it holds")
}
