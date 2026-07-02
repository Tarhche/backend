package profiler

import (
	"bytes"
	"context"
	"log/slog"
	"runtime/pprof"
	"time"
)

// lookupCollector periodically captures one of the runtime's named pprof
// profiles (heap, goroutine, mutex, ...). Every collector applies the shared
// rules: the adaptive sampler skips collections under CPU load, the resource
// guard enforces the per-minute and memory budgets, and the overhead monitor
// stretches the interval when producing profiles gets too expensive.
type lookupCollector struct {
	profile  string // runtime/pprof profile name passed to pprof.Lookup
	pt       ProfileType
	interval time.Duration

	monitor  *cpuMonitor
	sampler  *adaptiveSampler
	overhead *overheadMonitor
	guard    *resourceGuard
	inst     *instruments
	sink     func(context.Context, ProfileType, []byte)
	logger   *slog.Logger

	// onTick, when set, runs at every scheduled tick before the sampling
	// decision (e.g. the heap collector records GC pauses here so they are
	// captured even when the collection itself is skipped).
	onTick func(context.Context)
}

func newLookupCollector(
	profile string,
	pt ProfileType,
	interval time.Duration,
	cfg Config,
	monitor *cpuMonitor,
	guard *resourceGuard,
	inst *instruments,
	sink func(context.Context, ProfileType, []byte),
	logger *slog.Logger,
	onTick func(context.Context),
) *lookupCollector {
	return &lookupCollector{
		profile:  profile,
		pt:       pt,
		interval: interval,
		monitor:  monitor,
		sampler:  newAdaptiveSampler(cfg.CPULoadThreshold, cfg.MinSamplingRate),
		overhead: newOverheadMonitor(interval, cfg.MaxCPUPercent),
		guard:    guard,
		inst:     inst,
		sink:     sink,
		logger:   logger,
		onTick:   onTick,
	}
}

// run collects until ctx is cancelled. It is meant to be started once in its
// own goroutine.
func (c *lookupCollector) run(ctx context.Context) {
	c.inst.setInterval(c.pt, c.interval)

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
		c.inst.setInterval(c.pt, next)
		timer.Reset(next)
	}
}

func (c *lookupCollector) collect(ctx context.Context) {
	if c.onTick != nil {
		c.onTick(ctx)
	}

	if load := c.monitor.usage(); !c.sampler.shouldProfile(load) {
		c.inst.recordDropped(ctx, c.pt, reasonAdaptive)
		return
	}

	if err := c.guard.admit(); err != nil {
		c.inst.recordDropped(ctx, c.pt, reasonResourceLimit)
		return
	}

	started := time.Now()

	lookup := pprof.Lookup(c.profile)
	if lookup == nil {
		c.inst.recordDropped(ctx, c.pt, reasonEncodeError)
		c.logger.Warn("profiler: unknown runtime profile", slog.String("profile", c.profile))
		return
	}

	var buf bytes.Buffer
	if err := lookup.WriteTo(&buf, 0); err != nil {
		c.inst.recordDropped(ctx, c.pt, reasonEncodeError)
		c.logger.Warn("profiler: profile skipped", slog.String("profile", c.profile), slog.String("error", err.Error()))
		return
	}

	c.inst.recordCollected(ctx, c.pt)

	c.sink(ctx, c.pt, buf.Bytes())
	c.overhead.observe(time.Since(started))

	c.inst.recordCollection(ctx, c.pt, time.Since(started))
}
