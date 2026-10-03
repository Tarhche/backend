package presenter

import (
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// Runtime is one class a task may be run with, as the dashboard shows it to
// whoever is choosing: whether it is the default, whether anything can run it
// right now, what it can do, and how much room the nodes offering it have.
//
// The shape is the dashboard's contract (GET /api/dashboard/workload/runtimes
// and its "my" twin): the task form is built from it, so it changes only
// together with the frontend. Memory is in bytes, as it is everywhere in the
// workload.
type Runtime struct {
	Class   string `json:"class"`
	Default bool   `json:"default"`

	// Available says at least one healthy node offers the class; Nodes is
	// how many do.
	Available bool `json:"available"`
	Nodes     int  `json:"nodes"`

	Capabilities RuntimeCapabilities `json:"capabilities"`
	Capacity     RuntimeCapacity     `json:"capacity"`
}

// RuntimeCapabilities is what a class can do. The form turns off what a class
// cannot honour, rather than letting a task ask for it and fail.
type RuntimeCapabilities struct {
	Isolation       string   `json:"isolation"`
	NetworkPolicies []string `json:"network_policies"`
	StackNetworks   bool     `json:"stack_networks"`
	ReadOnlyRoot    bool     `json:"read_only_root"`
	DiskLimit       bool     `json:"disk_limit"`
	TTY             bool     `json:"tty"`
	RestartPolicies []string `json:"restart_policies"`
	MinMemory       uint64   `json:"min_memory"`
	MaxMemory       uint64   `json:"max_memory"`
	MaxCPU          float64  `json:"max_cpu"`
	Architectures   []string `json:"architectures"`
}

// RuntimeCapacity is what the nodes offering a class have for it between
// them, and how much of it is given out. Reserved says what a task is given is
// held for it, as a microVM's memory is, rather than shared.
type RuntimeCapacity struct {
	CPU             float64 `json:"cpu"`
	AllocatedCPU    float64 `json:"allocated_cpu"`
	Memory          uint64  `json:"memory"`
	AllocatedMemory uint64  `json:"allocated_memory"`
	Reserved        bool    `json:"reserved"`
}

// NewRuntime presents one class.
func NewRuntime(a runtime.Availability) Runtime {
	return Runtime{
		Class:     a.Class.String(),
		Default:   a.Default,
		Available: a.Available,
		Nodes:     a.Nodes,
		Capabilities: RuntimeCapabilities{
			Isolation:       a.Capabilities.Isolation,
			NetworkPolicies: policies(a.Capabilities.NetworkPolicies),
			StackNetworks:   a.Capabilities.StackNetworks,
			ReadOnlyRoot:    a.Capabilities.ReadOnlyRoot,
			DiskLimit:       a.Capabilities.DiskLimit,
			TTY:             a.Capabilities.TTY,
			RestartPolicies: list(a.Capabilities.RestartPolicies),
			MinMemory:       a.Capabilities.MinMemory,
			MaxMemory:       a.Capabilities.MaxMemory,
			MaxCPU:          a.Capabilities.MaxCPU,
			Architectures:   list(a.Capabilities.Architectures),
		},
		Capacity: RuntimeCapacity{
			CPU:             a.Capacity.CPU,
			AllocatedCPU:    a.Capacity.AllocatedCPU,
			Memory:          a.Capacity.Memory,
			AllocatedMemory: a.Capacity.AllocatedMemory,
			Reserved:        a.Capacity.Reserved,
		},
	}
}

// NewRuntimes presents every class, in the order the workload gave them.
func NewRuntimes(availability []runtime.Availability) []Runtime {
	items := make([]Runtime, len(availability))
	for i := range availability {
		items[i] = NewRuntime(availability[i])
	}

	return items
}

// policies is a class's network policies as words. A class nothing offers
// can do nothing, which is an empty list rather than no list at all: the
// dashboard reads every one of these as a list.
func policies(given []network.Policy) []string {
	words := make([]string, len(given))
	for i, p := range given {
		words[i] = p.String()
	}

	return words
}

// list is a list the dashboard can read as one, empty rather than absent.
func list(given []string) []string {
	if given == nil {
		return []string{}
	}

	return given
}
