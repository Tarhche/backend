// Package events is what the control plane and the nodes tell each other about
// VMs, over JetStream.
//
// Commands go from the control plane to a node, addressed to it by NodeName
// the way a task is scheduled: every node hears every command and acts only on
// its own. What becomes of a VM comes back in the node's heartbeats, and as
// results where there is one moment worth announcing.
package events

import (
	"maps"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Spec is a vm.Spec as it travels.
type Spec struct {
	ID             string            `json:"id"`
	Kind           vm.Kind           `json:"kind"`
	Image          string            `json:"image"`
	Resources      Resources         `json:"resources"`
	Ports          []port.Port       `json:"ports,omitempty"`
	Network        Network           `json:"network"`
	PersistentDisk bool              `json:"persistent_disk"`
	Labels         map[string]string `json:"labels,omitempty"`
	Command        []string          `json:"command,omitempty"`
	Env            []string          `json:"env,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
}

// Resources are vm.Resources as they travel: whole vCPUs, and bytes.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

// Network is a vm.Network as it travels.
type Network struct {
	Ingress vm.Access `json:"ingress"`
	Egress  vm.Access `json:"egress"`
}

// Info is a vm.Info as it travels: what a node offers, and how much of it is
// taken.
type Info struct {
	Engine    string    `json:"engine"`
	Version   string    `json:"version"`
	CPUs      uint      `json:"cpus"`
	Memory    uint64    `json:"memory"`
	Disk      uint64    `json:"disk"`
	Allocated Resources `json:"allocated"`
}

// Stats is a vm.Stats as it travels.
type Stats struct {
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskTotal   uint64    `json:"disk_total"`
	NetworkRx   uint64    `json:"network_rx"`
	NetworkTx   uint64    `json:"network_tx"`
	SampledAt   time.Time `json:"sampled_at"`
}

// NewSpec is a spec, ready to travel.
func NewSpec(s vm.Spec) Spec {
	return Spec{
		ID:             s.ID,
		Kind:           s.Kind,
		Image:          s.Image,
		Resources:      NewResources(s.Resources),
		Ports:          slices.Clone(s.Ports),
		Network:        Network{Ingress: s.Network.Ingress, Egress: s.Network.Egress},
		PersistentDisk: s.PersistentDisk,
		Labels:         maps.Clone(s.Labels),
		Command:        slices.Clone(s.Command),
		Env:            slices.Clone(s.Env),
		WorkingDir:     s.WorkingDir,
	}
}

// ToVM is the spec as the vm package has it.
func (s Spec) ToVM() vm.Spec {
	return vm.Spec{
		ID:             s.ID,
		Kind:           s.Kind,
		Image:          s.Image,
		Resources:      s.Resources.ToVM(),
		Ports:          slices.Clone(s.Ports),
		Network:        vm.Network{Ingress: s.Network.Ingress, Egress: s.Network.Egress},
		PersistentDisk: s.PersistentDisk,
		Labels:         maps.Clone(s.Labels),
		Command:        slices.Clone(s.Command),
		Env:            slices.Clone(s.Env),
		WorkingDir:     s.WorkingDir,
	}
}

// NewResources is what a VM is given, ready to travel.
func NewResources(r vm.Resources) Resources {
	return Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// ToVM is the resources as the vm package has them.
func (r Resources) ToVM() vm.Resources {
	return vm.Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// NewInfo is what a node offers, ready to travel.
func NewInfo(i vm.Info) Info {
	return Info{
		Engine:    i.Engine,
		Version:   i.Version,
		CPUs:      i.CPUs,
		Memory:    i.Memory,
		Disk:      i.Disk,
		Allocated: NewResources(i.Allocated),
	}
}

// ToVM is the info as the vm package has it.
func (i Info) ToVM() vm.Info {
	return vm.Info{
		Engine:    i.Engine,
		Version:   i.Version,
		CPUs:      i.CPUs,
		Memory:    i.Memory,
		Disk:      i.Disk,
		Allocated: i.Allocated.ToVM(),
	}
}

// NewStats is a sample, ready to travel.
func NewStats(s vm.Stats) Stats {
	return Stats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}

// ToVM is the sample as the vm package has it.
func (s Stats) ToVM() vm.Stats {
	return vm.Stats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}
