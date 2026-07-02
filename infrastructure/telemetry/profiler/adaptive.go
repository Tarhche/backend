package profiler

import (
	"math/rand/v2"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// cpuMonitor measures the process CPU usage (user+system) as a fraction of
// the machine's total capacity between consecutive calls to usage.
type cpuMonitor struct {
	mu       sync.Mutex
	lastCPU  time.Duration
	lastWall time.Time
}

func newCPUMonitor() *cpuMonitor {
	m := &cpuMonitor{}
	m.lastCPU = processCPUTime()
	m.lastWall = time.Now()

	return m
}

// usage returns the average CPU load (0..1) of this process since the
// previous call; the first call establishes the baseline and returns 0.
func (m *cpuMonitor) usage() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	cpu := processCPUTime()
	now := time.Now()

	wall := now.Sub(m.lastWall)
	if wall <= 0 {
		return 0
	}

	load := float64(cpu-m.lastCPU) / (float64(wall) * float64(runtime.NumCPU()))
	m.lastCPU = cpu
	m.lastWall = now

	return max(0, min(load, 1))
}

// processCPUTime returns the accumulated user+system CPU time of the process.
func processCPUTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}

	return time.Duration(ru.Utime.Nano()+ru.Stime.Nano()) * time.Nanosecond
}

// adaptiveSampler implements the blog post's load-based sampling: below the
// load threshold every scheduled collection runs; above it the sampling rate
// scales linearly from the base rate (1.0) down to the minimum rate so heavy
// load reduces, but never fully removes, profiling.
type adaptiveSampler struct {
	threshold float64
	baseRate  float64
	minRate   float64

	rand func() float64
}

func newAdaptiveSampler(threshold, minRate float64) *adaptiveSampler {
	return &adaptiveSampler{
		threshold: threshold,
		baseRate:  1.0,
		minRate:   minRate,
		rand:      rand.Float64,
	}
}

// rate returns the sampling probability for the given CPU load:
// current_rate = base_rate - (base_rate - min_rate) * excess_load_factor.
func (s *adaptiveSampler) rate(load float64) float64 {
	if load <= s.threshold {
		return s.baseRate
	}

	excess := (load - s.threshold) / (1 - s.threshold)
	excess = min(excess, 1)

	return max(s.baseRate-(s.baseRate-s.minRate)*excess, s.minRate)
}

// shouldProfile decides whether the collection scheduled now runs, given the
// current CPU load.
func (s *adaptiveSampler) shouldProfile(load float64) bool {
	return s.rand() < s.rate(load)
}

// overheadMonitor dynamically adjusts the collection interval so the time
// spent producing profiles stays below the target overhead ratio: the
// interval grows x1.5 (capped at 10x the base) when a collection was too
// expensive relative to the interval and shrinks x0.8 (floored at the base)
// once the overhead drops below half the target.
type overheadMonitor struct {
	target float64 // acceptable overhead ratio, e.g. 0.05 for 5%

	mu       sync.Mutex
	base     time.Duration
	max      time.Duration
	interval time.Duration
}

func newOverheadMonitor(base time.Duration, targetPercent float64) *overheadMonitor {
	return &overheadMonitor{
		target:   targetPercent / 100,
		base:     base,
		max:      10 * base,
		interval: base,
	}
}

// observe records the cost of the last collection and returns the interval to
// wait before the next one.
func (m *overheadMonitor) observe(cost time.Duration) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	overhead := float64(cost) / float64(m.interval)
	switch {
	case overhead > m.target:
		m.interval = min(m.interval*3/2, m.max)
	case overhead < m.target/2:
		m.interval = max(time.Duration(float64(m.interval)*0.8), m.base)
	}

	return m.interval
}

// current returns the interval without adjusting it.
func (m *overheadMonitor) current() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.interval
}
