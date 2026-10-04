package runs

import (
	"context"
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// Stats is what a running run's VM is using.
func (s *Supervisor) Stats(ctx context.Context, id string) (api.Stats, error) {
	s.mu.Lock()

	if err := s.readable(); err != nil {
		s.mu.Unlock()

		return api.Stats{}, err
	}

	r, found := s.runs[id]
	if !found {
		s.mu.Unlock()

		return api.Stats{}, notFound(id)
	}

	if r.live == nil {
		s.mu.Unlock()

		return api.Stats{}, notRunning(id)
	}

	memory := max(r.record.Spec.Memory, s.config.MemoryFloor)
	name := r.record.sandboxName()

	s.mu.Unlock()

	metrics, err := s.metrics(ctx)
	if err != nil {
		return api.Stats{}, err
	}

	return statsOf(metrics[name], memory), nil
}

// NodeStats is what a node's running runs are using between them, so that a
// node's heartbeat asks once rather than once for each of its runs.
func (s *Supervisor) NodeStats(ctx context.Context, node string) (api.Stats, error) {
	if len(node) == 0 {
		return api.Stats{}, newError(api.CodeInvalid, "node is required")
	}

	s.mu.Lock()

	if err := s.readable(); err != nil {
		s.mu.Unlock()

		return api.Stats{}, err
	}

	memories := make(map[string]uint64)
	for _, r := range s.runs {
		if r.live != nil && r.record.Spec.Node == node {
			memories[r.record.sandboxName()] = max(r.record.Spec.Memory, s.config.MemoryFloor)
		}
	}

	s.mu.Unlock()

	var total api.Stats

	if len(memories) == 0 {
		return total, nil
	}

	metrics, err := s.metrics(ctx)
	if err != nil {
		return api.Stats{}, err
	}

	for name, memory := range memories {
		stats := statsOf(metrics[name], memory)

		total.CPUPercent += stats.CPUPercent
		total.MemoryUsage += stats.MemoryUsage
		total.MemoryLimit += stats.MemoryLimit
		total.NetworkInput += stats.NetworkInput
		total.NetworkOutput += stats.NetworkOutput
		total.BlockInput += stats.BlockInput
		total.BlockOutput += stats.BlockOutput
	}

	return total, nil
}

// metrics is what every running VM is using, read for all of them at once
// and kept for a moment, so that every caller in that moment shares one
// reading.
func (s *Supervisor) metrics(ctx context.Context) (map[string]Metrics, error) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()

	if s.metricsValues != nil && time.Since(s.metricsAt) < s.config.MetricsTTL {
		return s.metricsValues, nil
	}

	callCtx, cancel := context.WithTimeout(ctx, s.config.CallTimeout)
	defer cancel()

	values, err := s.sandboxes.Metrics(callCtx)
	if err != nil {
		return nil, fmt.Errorf("the VMs' metrics could not be read: %w", err)
	}

	if values == nil {
		values = make(map[string]Metrics)
	}

	s.metricsValues = values
	s.metricsAt = time.Now()

	return values, nil
}

// statsOf is a VM's metrics as the contract reports them. A VM that has not
// reported a limit yet is limited to what it was given.
func statsOf(metrics Metrics, memory uint64) api.Stats {
	limit := metrics.MemoryLimit
	if limit == 0 {
		limit = memory
	}

	return api.Stats{
		CPUPercent:    metrics.CPUPercent,
		MemoryUsage:   metrics.MemoryUsage,
		MemoryLimit:   limit,
		NetworkInput:  metrics.NetRx,
		NetworkOutput: metrics.NetTx,
		BlockInput:    metrics.DiskRead,
		BlockOutput:   metrics.DiskWrite,
	}
}
