package presenter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const ingressDomain = "workload.example.com"

var at = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func running() vm.VM {
	return vm.VM{
		UUID:      "vm-uuid",
		Name:      "web",
		Slug:      "web-xkfqz",
		OwnerUUID: "owner-uuid",
		Kind:      vm.KindDocker,
		Image:     "docker:29-dind",
		Resources: vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:     []port.Port{80, 8080},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},

		PersistentDisk: true,
		Lifetime:       48 * time.Hour,
		ExpiresAt:      at.Add(48 * time.Hour),

		CurrentState:  vm.Running,
		ExpectedState: vm.Running,
		NodeName:      "orchestrator-01",
		Stats: vm.Stats{
			CPUPercent:  12.5,
			MemoryUsed:  512 << 20,
			MemoryLimit: 2 << 30,
			DiskUsed:    3 << 30,
			DiskTotal:   20 << 30,
			NetworkRx:   1024,
			NetworkTx:   2048,
			SampledAt:   at.Add(time.Minute),
		},
		RestoreFrom:     "never shown",
		LastHeartbeatAt: at.Add(time.Minute),
		CreatedAt:       at,
		StartedAt:       at.Add(10 * time.Second),
		UpdatedAt:       at.Add(time.Minute),
	}
}

func TestNewVM(t *testing.T) {
	t.Parallel()

	t.Run("a VM in the dashboard's words, every size in bytes and its lifetime in seconds", func(t *testing.T) {
		t.Parallel()

		owners := NewOwners([]user.User{{UUID: "owner-uuid", Name: "Mahdi", Username: "mahdi", Avatar: "avatar-uuid"}})

		presented, err := json.Marshal(NewVM(running(), ingressDomain, owners))
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"uuid": "vm-uuid",
			"name": "web",
			"slug": "web-xkfqz",
			"owner_uuid": "owner-uuid",
			"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
			"kind": "docker",
			"image": "docker:29-dind",
			"resources": {"cpus": 2, "memory": 2147483648, "disk": 21474836480},
			"ports": [80, 8080],
			"network": {"ingress": "allow", "egress": "deny"},
			"persistent_disk": true,
			"lifetime_seconds": 172800,
			"expires_at": "2026-10-06T12:00:00Z",
			"state": "running",
			"expected_state": "running",
			"node_name": "orchestrator-01",
			"stats": {
				"cpu_percent": 12.5,
				"memory_used": 536870912,
				"memory_limit": 2147483648,
				"disk_used": 3221225472,
				"disk_total": 21474836480,
				"network_rx": 1024,
				"network_tx": 2048,
				"sampled_at": "2026-10-04T12:01:00Z"
			},
			"urls": [
				{"port": 80, "url": "https://web-xkfqz-80.workload.example.com"},
				{"port": 8080, "url": "https://web-xkfqz-8080.workload.example.com"}
			],
			"created_at": "2026-10-04T12:00:00Z",
			"started_at": "2026-10-04T12:00:10Z",
			"updated_at": "2026-10-04T12:01:00Z"
		}`, string(presented))
	})

	t.Run("what has not happened yet is left out rather than dated year one", func(t *testing.T) {
		t.Parallel()

		created := vm.VM{
			UUID:          "vm-uuid",
			Name:          "box",
			OwnerUUID:     "gone-uuid",
			Kind:          vm.KindMachine,
			Network:       vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny},
			CurrentState:  vm.Created,
			ExpectedState: vm.Running,
			CreatedAt:     at,
		}

		presented, err := json.Marshal(NewVM(created, ingressDomain, NewOwners(nil)))
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"uuid": "vm-uuid",
			"name": "box",
			"slug": "",
			"owner_uuid": "gone-uuid",
			"kind": "machine",
			"image": "",
			"resources": {"cpus": 0, "memory": 0, "disk": 0},
			"ports": [],
			"network": {"ingress": "deny", "egress": "deny"},
			"persistent_disk": false,
			"lifetime_seconds": 0,
			"state": "created",
			"expected_state": "running",
			"urls": [],
			"created_at": "2026-10-04T12:00:00Z"
		}`, string(presented))
	})
}

func TestNewVM_run(t *testing.T) {
	t.Parallel()

	// a snippet the code runner is running, as the VM it runs in: the guest's,
	// and kept by the code runner.
	run := vm.VM{
		UUID:          "run-uuid",
		Name:          "01a10cd4-dae7-77d9-b400-7430fde9e001",
		Slug:          "01a10cd4-dae7-77d9-b400-7430fde9e001-nhxyb",
		OwnerUUID:     task.GuestOwnerUUID,
		Kind:          vm.KindMachine,
		Image:         "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
		Resources:     vm.Resources{CPUs: 2, Memory: 200 << 20, Disk: 100 << 20},
		Ports:         []port.Port{},
		Network:       vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Lifetime:      time.Minute,
		ExpiresAt:     at.Add(time.Second + time.Minute),
		CurrentState:  vm.Running,
		ExpectedState: vm.Running,
		NodeName:      "orchestrator-01",
		CreatedAt:     at,
		StartedAt:     at.Add(time.Second),
		UpdatedAt:     at.Add(time.Second),
		ManagedBy:     vm.ManagedByCodeRunner,
	}

	presented, err := json.Marshal(NewVM(run, ingressDomain, NewOwners(nil)))
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"uuid": "run-uuid",
		"name": "01a10cd4-dae7-77d9-b400-7430fde9e001",
		"slug": "01a10cd4-dae7-77d9-b400-7430fde9e001-nhxyb",
		"owner_uuid": "guest",
		"owner": {"uuid": "guest"},
		"kind": "machine",
		"image": "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
		"resources": {"cpus": 2, "memory": 209715200, "disk": 104857600},
		"ports": [],
		"network": {"ingress": "allow", "egress": "deny"},
		"persistent_disk": false,
		"lifetime_seconds": 60,
		"expires_at": "2026-10-04T12:01:01Z",
		"state": "running",
		"expected_state": "running",
		"node_name": "orchestrator-01",
		"urls": [],
		"created_at": "2026-10-04T12:00:00Z",
		"started_at": "2026-10-04T12:00:01Z",
		"updated_at": "2026-10-04T12:00:01Z",
		"managed_by": "code-runner"
	}`, string(presented))
}

func TestNewURLs(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name   string
		vm     func(v *vm.VM)
		domain string
		want   []URL
	}{
		{
			name:   "every port answers on the slug with the port appended, over TLS",
			domain: "workload.example.com",
			want: []URL{
				{Port: 80, URL: "https://web-xkfqz-80.workload.example.com"},
				{Port: 8080, URL: "https://web-xkfqz-8080.workload.example.com"},
			},
		},
		{
			name:   "under localhost there is no certificate, so it is plain http",
			domain: "workload.localhost:8030",
			want: []URL{
				{Port: 80, URL: "http://web-xkfqz-80.workload.localhost:8030"},
				{Port: 8080, URL: "http://web-xkfqz-8080.workload.localhost:8030"},
			},
		},
		{
			name:   "with or without a port",
			domain: "workload.localhost",
			want: []URL{
				{Port: 80, URL: "http://web-xkfqz-80.workload.localhost"},
				{Port: 8080, URL: "http://web-xkfqz-8080.workload.localhost"},
			},
		},
		{
			name:   "a domain that only looks like localhost is somebody else's, over TLS",
			domain: "localhost.example.com:8443",
			want: []URL{
				{Port: 80, URL: "https://web-xkfqz-80.localhost.example.com:8443"},
				{Port: 8080, URL: "https://web-xkfqz-8080.localhost.example.com:8443"},
			},
		},
		{
			name:   "a VM whose ingress is denied is served nowhere, whatever ports it lists",
			vm:     func(v *vm.VM) { v.Network.Ingress = vm.AccessDeny },
			domain: "workload.example.com",
			want:   []URL{},
		},
		{
			name:   "a VM with no slug yet has no address to give",
			vm:     func(v *vm.VM) { v.Slug = "" },
			domain: "workload.example.com",
			want:   []URL{},
		},
		{
			name:   "a VM exposing nothing has no addresses",
			vm:     func(v *vm.VM) { v.Ports = nil },
			domain: "workload.example.com",
			want:   []URL{},
		},
		{
			name:   "with no ingress domain configured there is nowhere to point",
			domain: "",
			want:   []URL{},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := running()
			if tt.vm != nil {
				tt.vm(&v)
			}

			assert.Equal(t, tt.want, NewURLs(v, tt.domain))
		})
	}
}

func TestNewVMs(t *testing.T) {
	t.Parallel()

	assert.Empty(t, NewVMs(nil, ingressDomain, NewOwners(nil)))

	presented := NewVMs([]vm.VM{running(), {UUID: "other-uuid", OwnerUUID: "owner-uuid"}}, ingressDomain, NewOwners(nil))
	require.Len(t, presented, 2)
	assert.Equal(t, "vm-uuid", presented[0].UUID)
	assert.Equal(t, "other-uuid", presented[1].UUID)
}
