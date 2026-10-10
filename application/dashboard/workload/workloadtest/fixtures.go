package workloadtest

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// At is when everything in these fixtures happened.
var At = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

// VM is a running Docker VM of OwnerUUID's, with two ports the ingress
// serves.
func VM() vm.VM {
	return vm.VM{
		UUID:           "vm-uuid",
		Name:           "docker-1",
		Slug:           "docker-1-xkfqz",
		OwnerUUID:      OwnerUUID,
		Kind:           vm.KindDocker,
		Image:          "docker:29-dind",
		Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:          []port.Port{80, 8080},
		Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		PersistentDisk: true,
		CurrentState:   vm.Running,
		ExpectedState:  vm.Running,
		NodeName:       "orchestrator-01",
		CreatedAt:      At,
		StartedAt:      At.Add(time.Minute),
	}
}

// VMJSON is VM as the dashboard shows it.
const VMJSON = `{
	"uuid": "vm-uuid",
	"name": "docker-1",
	"slug": "docker-1-xkfqz",
	"owner_uuid": "owner-uuid",
	"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
	"kind": "docker",
	"image": "docker:29-dind",
	"resources": {"cpus": 2, "memory": 2147483648, "disk": 21474836480},
	"ports": [80, 8080],
	"network": {"ingress": "allow", "egress": "allow"},
	"persistent_disk": true,
	"lifetime_seconds": 0,
	"state": "running",
	"expected_state": "running",
	"node_name": "orchestrator-01",
	"urls": [
		{"port": 80, "url": "https://docker-1-xkfqz-80.workload.example.com"},
		{"port": 8080, "url": "https://docker-1-xkfqz-8080.workload.example.com"}
	],
	"created_at": "2026-10-04T12:00:00Z",
	"started_at": "2026-10-04T12:01:00Z"
}`

// Snapshot is a ready snapshot of VM.
func Snapshot() snapshot.Snapshot {
	return snapshot.Snapshot{
		UUID:        "snapshot-uuid",
		Name:        "before the upgrade",
		OwnerUUID:   OwnerUUID,
		VMUUID:      "vm-uuid",
		VMName:      "docker-1",
		Kind:        vm.KindDocker,
		Image:       "docker:29-dind",
		Disk:        20 << 30,
		Engine:      "microsandbox/0.7.2",
		Size:        1 << 30,
		State:       snapshot.Ready,
		CreatedAt:   At,
		CompletedAt: At.Add(time.Minute),
	}
}

// SnapshotJSON is Snapshot as the dashboard shows it.
const SnapshotJSON = `{
	"uuid": "snapshot-uuid",
	"name": "before the upgrade",
	"owner_uuid": "owner-uuid",
	"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
	"vm_uuid": "vm-uuid",
	"vm_name": "docker-1",
	"kind": "docker",
	"image": "docker:29-dind",
	"disk": 21474836480,
	"engine": "microsandbox/0.7.2",
	"size": 1073741824,
	"state": "ready",
	"created_at": "2026-10-04T12:00:00Z",
	"completed_at": "2026-10-04T12:01:00Z"
}`

// Container is a container of VM, deployed by Stack.
func Container() docker.Container {
	return docker.Container{
		ID:        "c0ffee",
		Name:      "shop-abcde-web-1",
		Image:     "nginx:1.27",
		State:     "running",
		Status:    "Up 3 minutes",
		Command:   "nginx -g 'daemon off;'",
		Ports:     []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
		Networks:  []string{"shop-abcde_default"},
		Stack:     "shop-abcde",
		Service:   "web",
		CreatedAt: At,
	}
}

// ContainerJSON is Container as the dashboard shows it.
const ContainerJSON = `{
	"id": "c0ffee",
	"name": "shop-abcde-web-1",
	"image": "nginx:1.27",
	"state": "running",
	"status": "Up 3 minutes",
	"command": "nginx -g 'daemon off;'",
	"ports": [{"container_port": 80, "host_port": 8080, "protocol": "tcp"}],
	"networks": ["shop-abcde_default"],
	"mounts": [],
	"stack": "shop-abcde",
	"service": "web",
	"created_at": "2026-10-04T12:00:00Z"
}`

// Stack is a running stack of OwnerUUID's in VM.
func Stack() stack.Stack {
	return stack.Stack{
		UUID:          "stack-uuid",
		Name:          "shop",
		OwnerUUID:     OwnerUUID,
		VMUUID:        "vm-uuid",
		VMName:        "docker-1",
		Slug:          "shop-abcde",
		Compose:       "services:\n  web:\n    image: nginx:1.27\n",
		ExpectedState: stack.Running,
		State:         stack.Running,
		Output:        "Container shop-abcde-web-1  Started",
		CreatedAt:     At,
	}
}

// StackJSON is Stack as the dashboard shows it, compose file and all.
const StackJSON = `{
	"uuid": "stack-uuid",
	"name": "shop",
	"slug": "shop-abcde",
	"owner_uuid": "owner-uuid",
	"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
	"vm_uuid": "vm-uuid",
	"vm_name": "docker-1",
	"compose": "services:\n  web:\n    image: nginx:1.27\n",
	"state": "running",
	"expected_state": "running",
	"output": "Container shop-abcde-web-1  Started",
	"created_at": "2026-10-04T12:00:00Z"
}`
