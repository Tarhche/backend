// Package node says what a node offers and how much of it is taken, as its
// engine reports it.
package node

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Manager reads a node's stats off its engine.
//
// What a node is asked for is placed by what it has given away, not by what
// is busy this second: a VM given four vCPUs may take them all at any moment.
// So the CPU percent is the vCPUs the instances on the node were given, of
// the vCPUs it offers, and goes past 100 when CPUs are given more than once,
// which the control plane allows; memory is the bytes given, of the bytes
// offered, which never are.
type Manager struct {
	engine vm.Engine
}

var _ node.Manager = &Manager{}

func NewManager(engine vm.Engine) *Manager {
	return &Manager{engine: engine}
}

// Stats is what the node's engine says it offers and has given. The engine is
// this node's alone, so the name says nothing the engine does not.
func (m *Manager) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	info, err := m.engine.Info(ctx)
	if err != nil {
		return node.Stats{}, err
	}

	stats := node.Stats{
		MemoryUsage: info.Allocated.Memory,
		MemoryLimit: info.Memory,
	}

	if info.CPUs > 0 {
		stats.CPUPercent = float64(info.Allocated.CPUs) / float64(info.CPUs) * 100
	}

	if info.Memory > 0 {
		stats.MemoryPercent = float64(info.Allocated.Memory) / float64(info.Memory) * 100
	}

	return stats, nil
}
