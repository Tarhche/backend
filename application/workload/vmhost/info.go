package vmhost

import (
	"context"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Info is what vmhost says about itself: what it is, whether it can run VMs
// right now, what the class it stands behind can do, and how much of its
// budget is taken. It is asked on every node heartbeat, so it is answered from
// memory.
func (e *Engine) Info(ctx context.Context) (vm.Info, error) {
	all, err := e.states.All(ctx)
	if err != nil {
		return vm.Info{}, err
	}

	healthy, reason := e.Health()

	return vm.Info{
		Version:           e.config.Version,
		Hypervisor:        e.hypervisor.Name(),
		HypervisorVersion: e.hypervisor.Version(),
		GuestVersion:      guest.ProtocolVersion,
		Architecture:      e.config.Architecture,
		ProcessMode:       e.config.ProcessMode,
		Healthy:           healthy,
		Reason:            reason,
		Capabilities:      e.capabilities(),
		Capacity:          e.capacity(all),
	}, nil
}

// capabilities are what a VM here can be: a microVM on no network, an
// isolated one or a public one, in a stack of its own, with a root it cannot
// write to, held to its disk, with a terminal, under any of compose's restart
// policies, and within the size one VM may be.
func (e *Engine) capabilities() runtime.Capabilities {
	var architectures []string
	if len(e.config.Architecture) > 0 {
		architectures = []string{e.config.Architecture}
	}

	return runtime.Capabilities{
		Isolation:       runtime.IsolationMicroVM,
		NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
		StackNetworks:   true,
		ReadOnlyRoot:    true,
		DiskLimit:       true,
		TTY:             true,
		RestartPolicies: slices.Clone(vm.RestartPolicies),
		MinMemory:       e.config.MinMemory,
		MaxMemory:       e.config.MaxVMMemory,
		MaxCPU:          e.config.MaxVMCPU,
		Architectures:   architectures,
	}
}

// capacity is vmhost's budget, and how much of it the VMs it holds have
// taken: memory with each VM's VMM overhead, as admission counts it; CPUs as
// whole ones, out of the host's times the overcommit; and the scratch disks
// promised, out of what the free disk may be promised. What a VM is given is
// held for it whether it uses it or not.
func (e *Engine) capacity(all []vm.VM) runtime.Capacity {
	capacity := runtime.Capacity{
		CPU:      float64(e.config.HostCPUs) * e.config.CPUOvercommit,
		Memory:   e.config.MaxMemory,
		Reserved: true,
	}

	for _, v := range all {
		if holdsRoom(v.State) {
			capacity.AllocatedMemory += vm.MemoryBytes(v.MemoryMiB) + e.config.MemoryOverhead
			capacity.AllocatedCPU += float64(v.VCPUs)
		}

		if !v.Spec.ReadOnly && v.State != vm.StateRemoving {
			capacity.AllocatedDisk += vm.ScratchSize(v.Spec.Resources)
		}
	}

	if e.freeSpace != nil {
		if free, err := e.freeSpace(); err == nil && free > e.config.DiskReserve {
			capacity.Disk = uint64(float64(free) * e.config.DiskOvercommit)
		}
	}

	return capacity
}

// reserved is the memory the VMs hold, VMM overhead included.
func (e *Engine) reserved(all []vm.VM) uint64 {
	var memory uint64

	for _, v := range all {
		if holdsRoom(v.State) {
			memory += vm.MemoryBytes(v.MemoryMiB) + e.config.MemoryOverhead
		}
	}

	return memory
}
