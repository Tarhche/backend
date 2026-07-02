package profiler

import (
	"bytes"
	"context"
	"log/slog"
	"runtime/pprof"
	"time"
)

// cpuCollector periodically captures CPU profiles: every interval it samples
// the runtime for the configured window and hands the pprof payload to sink.
// The interval is adjusted by the overhead monitor and collections are
// skipped by the adaptive sampler under load and by the resource guard when
// budgets are exhausted.
type cpuCollector struct {
	interval time.Duration
	duration time.Duration

	monitor  *cpuMonitor
	sampler  *adaptiveSampler
	overhead *overheadMonitor
	guard    *resourceGuard
	inst     *instruments
	sink     func(context.Context, ProfileType, []byte)
	logger   *slog.Logger
}

func newCPUCollector(
	cfg Config,
	monitor *cpuMonitor,
	guard *resourceGuard,
	inst *instruments,
	sink func(context.Context, ProfileType, []byte),
	logger *slog.Logger,
) *cpuCollector {
	return &cpuCollector{
		interval: cfg.CPUInterval,
		duration: cfg.CPUDuration,
		monitor:  monitor,
		sampler:  newAdaptiveSampler(cfg.CPULoadThreshold, cfg.MinSamplingRate),
		overhead: newOverheadMonitor(cfg.CPUInterval, cfg.MaxCPUPercent),
		guard:    guard,
		inst:     inst,
		sink:     sink,
		logger:   logger,
	}
}

// run collects until ctx is cancelled. It is meant to be started once in its
// own goroutine.
func (c *cpuCollector) run(ctx context.Context) {
	c.inst.setInterval(ProfileTypeCPU, c.interval)

	timer := time.NewTimer(c.interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		c.collect(ctx)

		next := c.overhead.current()
		c.inst.setInterval(ProfileTypeCPU, next)
		timer.Reset(next)
	}
}

func (c *cpuCollector) collect(ctx context.Context) {
	if load := c.monitor.usage(); !c.sampler.shouldProfile(load) {
		c.inst.recordDropped(ctx, ProfileTypeCPU, reasonAdaptive)
		return
	}

	if err := c.guard.admit(); err != nil {
		c.inst.recordDropped(ctx, ProfileTypeCPU, reasonResourceLimit)
		return
	}

	started := time.Now()

	var buf bytes.Buffer
	if err := pprof.StartCPUProfile(&buf); err != nil {
		// most likely another CPU profile is running (e.g. net/http/pprof)
		c.inst.recordDropped(ctx, ProfileTypeCPU, reasonContention)
		c.logger.Warn("profiler: cpu profile skipped", slog.String("error", err.Error()))
		return
	}

	select {
	case <-time.After(c.duration):
	case <-ctx.Done():
	}
	pprof.StopCPUProfile()

	if ctx.Err() != nil {
		return
	}

	c.inst.recordCollected(ctx, ProfileTypeCPU)

	// only the processing cost counts as overhead; the sampling window
	// itself is passive and its runtime cost is governed by the adaptive
	// sampler above.
	processingStart := time.Now()
	c.sink(ctx, ProfileTypeCPU, buf.Bytes())
	c.overhead.observe(time.Since(processingStart))

	c.inst.recordCollection(ctx, ProfileTypeCPU, time.Since(started))
}
