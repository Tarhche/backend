package vms

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// VMBson is a VM as it is stored.
//
// Nothing that can be cleared is omitempty: an update sets every field it
// carries, and a field left out of it keeps whatever it held before, so a
// reason that was cleared, or an expiry that was lifted, would come back.
type VMBson struct {
	UUID           string        `bson:"_id,omitempty"`
	Name           string        `bson:"name"`
	Slug           string        `bson:"slug"`
	OwnerUUID      string        `bson:"owner_uuid"`
	Kind           string        `bson:"kind"`
	Image          string        `bson:"image"`
	Resources      Resources     `bson:"resources"`
	Ports          []port.Port   `bson:"ports"`
	Network        Network       `bson:"network"`
	PersistentDisk bool          `bson:"persistent_disk"`
	Lifetime       time.Duration `bson:"lifetime"`
	ExpiresAt      time.Time     `bson:"expires_at"`
	CurrentState   uint          `bson:"current_state"`
	ExpectedState  uint          `bson:"expected_state"`
	Reason         string        `bson:"reason"`
	NodeName       string        `bson:"node_name"`
	Stats          Stats         `bson:"stats"`
	RestoreFrom    string        `bson:"restore_from"`

	LastHeartbeatAt time.Time `bson:"last_heartbeat_at"`
	CreatedAt       time.Time `bson:"created_at"`
	StartedAt       time.Time `bson:"started_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

// Resources are whole vCPUs and bytes, as they are everywhere in the workload.
type Resources struct {
	CPUs   uint   `bson:"cpus"`
	Memory uint64 `bson:"memory"`
	Disk   uint64 `bson:"disk"`
}

type Network struct {
	Ingress string `bson:"ingress"`
	Egress  string `bson:"egress"`
}

type Stats struct {
	CPUPercent  float64   `bson:"cpu_percent"`
	MemoryUsed  uint64    `bson:"memory_used"`
	MemoryLimit uint64    `bson:"memory_limit"`
	DiskUsed    uint64    `bson:"disk_used"`
	DiskTotal   uint64    `bson:"disk_total"`
	NetworkRx   uint64    `bson:"network_rx"`
	NetworkTx   uint64    `bson:"network_tx"`
	SampledAt   time.Time `bson:"sampled_at"`
}

// toVM reads a stored VM back. Every read goes through here, so a field added
// to the model reaches the domain from one place.
func toVM(v *VMBson) vm.VM {
	return vm.VM{
		UUID:      v.UUID,
		Name:      v.Name,
		Slug:      v.Slug,
		OwnerUUID: v.OwnerUUID,
		Kind:      vm.Kind(v.Kind),
		Image:     v.Image,
		Resources: vm.Resources{
			CPUs:   v.Resources.CPUs,
			Memory: v.Resources.Memory,
			Disk:   v.Resources.Disk,
		},
		Ports: v.Ports,
		Network: vm.Network{
			Ingress: vm.Access(v.Network.Ingress),
			Egress:  vm.Access(v.Network.Egress),
		},
		PersistentDisk: v.PersistentDisk,
		Lifetime:       v.Lifetime,
		ExpiresAt:      v.ExpiresAt,
		CurrentState:   vm.State(v.CurrentState),
		ExpectedState:  vm.State(v.ExpectedState),
		Reason:         v.Reason,
		NodeName:       v.NodeName,
		Stats: vm.Stats{
			CPUPercent:  v.Stats.CPUPercent,
			MemoryUsed:  v.Stats.MemoryUsed,
			MemoryLimit: v.Stats.MemoryLimit,
			DiskUsed:    v.Stats.DiskUsed,
			DiskTotal:   v.Stats.DiskTotal,
			NetworkRx:   v.Stats.NetworkRx,
			NetworkTx:   v.Stats.NetworkTx,
			SampledAt:   v.Stats.SampledAt,
		},
		RestoreFrom:     v.RestoreFrom,
		LastHeartbeatAt: v.LastHeartbeatAt,
		CreatedAt:       v.CreatedAt,
		StartedAt:       v.StartedAt,
		UpdatedAt:       v.UpdatedAt,
	}
}

// toBson prepares a VM to be stored.
func toBson(v *vm.VM) VMBson {
	ports := v.Ports
	if ports == nil {
		ports = []port.Port{}
	}

	return VMBson{
		UUID:      v.UUID,
		Name:      v.Name,
		Slug:      v.Slug,
		OwnerUUID: v.OwnerUUID,
		Kind:      string(v.Kind),
		Image:     v.Image,
		Resources: Resources{
			CPUs:   v.Resources.CPUs,
			Memory: v.Resources.Memory,
			Disk:   v.Resources.Disk,
		},
		Ports: ports,
		Network: Network{
			Ingress: string(v.Network.Ingress),
			Egress:  string(v.Network.Egress),
		},
		PersistentDisk: v.PersistentDisk,
		Lifetime:       v.Lifetime,
		ExpiresAt:      v.ExpiresAt,
		CurrentState:   uint(v.CurrentState),
		ExpectedState:  uint(v.ExpectedState),
		Reason:         v.Reason,
		NodeName:       v.NodeName,
		Stats: Stats{
			CPUPercent:  v.Stats.CPUPercent,
			MemoryUsed:  v.Stats.MemoryUsed,
			MemoryLimit: v.Stats.MemoryLimit,
			DiskUsed:    v.Stats.DiskUsed,
			DiskTotal:   v.Stats.DiskTotal,
			NetworkRx:   v.Stats.NetworkRx,
			NetworkTx:   v.Stats.NetworkTx,
			SampledAt:   v.Stats.SampledAt,
		},
		RestoreFrom:     v.RestoreFrom,
		LastHeartbeatAt: v.LastHeartbeatAt,
		CreatedAt:       v.CreatedAt,
		StartedAt:       v.StartedAt,
		UpdatedAt:       v.UpdatedAt,
	}
}
