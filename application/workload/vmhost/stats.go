package vmhost

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// sample is what a machine had used, all told, when it was last measured,
// and when.
type sample struct {
	at    time.Time
	usage vm.Usage
}

// Stats is what a VM uses right now. A VM that does not run uses nothing, as a
// stopped container does.
//
// Memory is the guest's own view of it, since the host's view of a VM's memory
// only ever grows, held against what the machine was given. Where the host
// can count the rest — the machine's own cgroup, its taps — it is what the
// host counted: CPU measured between two askings, as docker measures it, VMM
// included; otherwise it is what the guest says.
func (e *Engine) Stats(ctx context.Context, id string) (vm.Stats, error) {
	v, err := e.states.Get(ctx, id)
	if err != nil {
		return vm.Stats{}, err
	}

	k := e.keeper(id)
	if k == nil || v.State != vm.StateRunning {
		return vm.Stats{}, nil
	}

	used, err := k.client.Stats(ctx)
	if err != nil {
		e.metrics.agentFailed(ctx, "stats")

		return vm.Stats{}, guestError(err)
	}

	stats := vm.Stats{
		PIDs:          used.PIDs,
		CPUPercent:    used.CPUPercent,
		MemoryUsage:   used.MemoryUsage,
		MemoryLimit:   vm.MemoryBytes(v.MemoryMiB),
		NetworkInput:  used.NetworkInput,
		NetworkOutput: used.NetworkOutput,
		BlockInput:    used.BlockInput,
		BlockOutput:   used.BlockOutput,
	}

	e.measure(ctx, v, &stats)

	return stats, nil
}

// measure puts what the host counted for a VM's machine in place of what its
// guest said, where the host can count it.
func (e *Engine) measure(ctx context.Context, v vm.VM, stats *vm.Stats) {
	if e.usage == nil {
		return
	}

	machine, err := e.hypervisor.Machine(ctx, v.ID)
	if err != nil || !machine.Running {
		return
	}

	devices := make([]string, 0, len(v.Interfaces))
	for _, i := range v.Interfaces {
		devices = append(devices, i.Device)
	}

	usage, err := e.usage.Usage(ctx, machine, devices)
	if err != nil {
		e.logger.DebugContext(ctx, "what a vm's machine uses cannot be counted on the host", "vm", v.ID, "error", err)

		return
	}

	now := time.Now()

	e.lock.Lock()
	previous, measured := e.samples[v.ID]
	e.samples[v.ID] = sample{at: now, usage: usage}
	e.lock.Unlock()

	stats.NetworkInput = usage.NetworkInput
	stats.NetworkOutput = usage.NetworkOutput
	stats.BlockInput = usage.BlockInput
	stats.BlockOutput = usage.BlockOutput

	// CPU is what the machine used between two askings; the first asking has
	// nothing to measure against, and keeps the guest's view.
	elapsed := now.Sub(previous.at)
	if measured && elapsed > 0 && usage.CPU >= previous.usage.CPU {
		stats.CPUPercent = float64(usage.CPU-previous.usage.CPU) / float64(elapsed) * 100
	}
}

// forgetSample lets go of what a machine was last measured at, once it is
// gone.
func (e *Engine) forgetSample(id string) {
	e.lock.Lock()
	defer e.lock.Unlock()

	delete(e.samples, id)
}
