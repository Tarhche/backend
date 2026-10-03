package nodes

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

type NodeBson struct {
	Name  string `bson:"name"`
	Role  string `bson:"role"`
	Stats Stats  `bson:"stats"`

	// Runtimes are the classes the node offers, as its last heartbeat said.
	// It is written whole on every heartbeat, empty included: a class the
	// node no longer offers is gone, rather than left behind from an earlier
	// heartbeat for placement to trust.
	Runtimes []Offer `bson:"runtimes"`

	LastHeartbeatAt time.Time `bson:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time `bson:"created_at,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at,omitempty"`
}

type Stats struct {
	PIDs          uint64  `bson:"pids"`
	CPUPercent    float64 `bson:"cpu_percent"`
	MemoryUsage   uint64  `bson:"memory_usage"`
	MemoryLimit   uint64  `bson:"memory_limit,omitempty"`
	MemoryPercent float64 `bson:"memory_percent"`
	NetworkInput  uint64  `bson:"network_input"`
	NetworkOutput uint64  `bson:"network_output"`
	BlockInput    uint64  `bson:"block_input"`
	BlockOutput   uint64  `bson:"block_output"`
}

// Offer is one class as a node runs it.
type Offer struct {
	Class        string       `bson:"class"`
	Driver       string       `bson:"driver,omitempty"`
	Version      string       `bson:"version,omitempty"`
	Healthy      bool         `bson:"healthy"`
	Reason       string       `bson:"reason,omitempty"`
	Capabilities Capabilities `bson:"capabilities"`
	Capacity     Capacity     `bson:"capacity"`
}

// Capabilities are what a class can do on the node. Memory is in bytes.
type Capabilities struct {
	Isolation       string   `bson:"isolation,omitempty"`
	NetworkPolicies []string `bson:"network_policies,omitempty"`
	StackNetworks   bool     `bson:"stack_networks"`
	ReadOnlyRoot    bool     `bson:"read_only_root"`
	DiskLimit       bool     `bson:"disk_limit"`
	TTY             bool     `bson:"tty"`
	RestartPolicies []string `bson:"restart_policies,omitempty"`
	MinMemory       uint64   `bson:"min_memory,omitempty"`
	MaxMemory       uint64   `bson:"max_memory,omitempty"`
	MaxCPU          float64  `bson:"max_cpu,omitempty"`
	Architectures   []string `bson:"architectures,omitempty"`
}

// Capacity is what the node has for the class. Memory and disk are in bytes.
type Capacity struct {
	CPU             float64 `bson:"cpu"`
	AllocatedCPU    float64 `bson:"allocated_cpu"`
	Memory          uint64  `bson:"memory"`
	AllocatedMemory uint64  `bson:"allocated_memory"`
	Disk            uint64  `bson:"disk,omitempty"`
	AllocatedDisk   uint64  `bson:"allocated_disk,omitempty"`
	Reserved        bool    `bson:"reserved"`
}

// toNode reads a stored node back. Every read goes through here, so a field
// added to the model reaches the domain from one place.
func toNode(n *NodeBson) node.Node {
	return node.Node{
		Name: n.Name,
		Role: node.Role(n.Role),
		Stats: node.Stats{
			PIDs:          n.Stats.PIDs,
			CPUPercent:    n.Stats.CPUPercent,
			MemoryUsage:   n.Stats.MemoryUsage,
			MemoryLimit:   n.Stats.MemoryLimit,
			MemoryPercent: n.Stats.MemoryPercent,
			NetworkInput:  n.Stats.NetworkInput,
			NetworkOutput: n.Stats.NetworkOutput,
			BlockInput:    n.Stats.BlockInput,
			BlockOutput:   n.Stats.BlockOutput,
		},
		Runtimes:        toOffers(n.Runtimes),
		LastHeartbeatAt: n.LastHeartbeatAt,
	}
}

// toBson prepares a node to be stored. When it is written is the
// repository's to say.
func toBson(n *node.Node) NodeBson {
	return NodeBson{
		Name: n.Name,
		Role: string(n.Role),
		Stats: Stats{
			PIDs:          n.Stats.PIDs,
			CPUPercent:    n.Stats.CPUPercent,
			MemoryUsage:   n.Stats.MemoryUsage,
			MemoryLimit:   n.Stats.MemoryLimit,
			MemoryPercent: n.Stats.MemoryPercent,
			NetworkInput:  n.Stats.NetworkInput,
			NetworkOutput: n.Stats.NetworkOutput,
			BlockInput:    n.Stats.BlockInput,
			BlockOutput:   n.Stats.BlockOutput,
		},
		Runtimes:        fromOffers(n.Runtimes),
		LastHeartbeatAt: n.LastHeartbeatAt,
	}
}

// toOffers reads a node's offers back. A node that has said nothing about
// classes is read as offering none it has said, which is what placement takes
// for a node from before there were classes.
func toOffers(stored []Offer) []runtime.Offer {
	if len(stored) == 0 {
		return nil
	}

	offers := make([]runtime.Offer, len(stored))
	for i, o := range stored {
		policies := make([]network.Policy, len(o.Capabilities.NetworkPolicies))
		for j, policy := range o.Capabilities.NetworkPolicies {
			policies[j] = network.Policy(policy)
		}

		offers[i] = runtime.Offer{
			Class:   runtime.Class(o.Class),
			Driver:  o.Driver,
			Version: o.Version,
			Healthy: o.Healthy,
			Reason:  o.Reason,
			Capabilities: runtime.Capabilities{
				Isolation:       o.Capabilities.Isolation,
				NetworkPolicies: policies,
				StackNetworks:   o.Capabilities.StackNetworks,
				ReadOnlyRoot:    o.Capabilities.ReadOnlyRoot,
				DiskLimit:       o.Capabilities.DiskLimit,
				TTY:             o.Capabilities.TTY,
				RestartPolicies: o.Capabilities.RestartPolicies,
				MinMemory:       o.Capabilities.MinMemory,
				MaxMemory:       o.Capabilities.MaxMemory,
				MaxCPU:          o.Capabilities.MaxCPU,
				Architectures:   o.Capabilities.Architectures,
			},
			Capacity: runtime.Capacity{
				CPU:             o.Capacity.CPU,
				AllocatedCPU:    o.Capacity.AllocatedCPU,
				Memory:          o.Capacity.Memory,
				AllocatedMemory: o.Capacity.AllocatedMemory,
				Disk:            o.Capacity.Disk,
				AllocatedDisk:   o.Capacity.AllocatedDisk,
				Reserved:        o.Capacity.Reserved,
			},
		}
	}

	return offers
}

func fromOffers(offers []runtime.Offer) []Offer {
	if len(offers) == 0 {
		return nil
	}

	stored := make([]Offer, len(offers))
	for i, o := range offers {
		policies := make([]string, len(o.Capabilities.NetworkPolicies))
		for j, policy := range o.Capabilities.NetworkPolicies {
			policies[j] = policy.String()
		}

		stored[i] = Offer{
			Class:   o.Class.String(),
			Driver:  o.Driver,
			Version: o.Version,
			Healthy: o.Healthy,
			Reason:  o.Reason,
			Capabilities: Capabilities{
				Isolation:       o.Capabilities.Isolation,
				NetworkPolicies: policies,
				StackNetworks:   o.Capabilities.StackNetworks,
				ReadOnlyRoot:    o.Capabilities.ReadOnlyRoot,
				DiskLimit:       o.Capabilities.DiskLimit,
				TTY:             o.Capabilities.TTY,
				RestartPolicies: o.Capabilities.RestartPolicies,
				MinMemory:       o.Capabilities.MinMemory,
				MaxMemory:       o.Capabilities.MaxMemory,
				MaxCPU:          o.Capabilities.MaxCPU,
				Architectures:   o.Capabilities.Architectures,
			},
			Capacity: Capacity{
				CPU:             o.Capacity.CPU,
				AllocatedCPU:    o.Capacity.AllocatedCPU,
				Memory:          o.Capacity.Memory,
				AllocatedMemory: o.Capacity.AllocatedMemory,
				Disk:            o.Capacity.Disk,
				AllocatedDisk:   o.Capacity.AllocatedDisk,
				Reserved:        o.Capacity.Reserved,
			},
		}
	}

	return stored
}
