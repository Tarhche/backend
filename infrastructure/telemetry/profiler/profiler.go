package profiler

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/google/pprof/profile"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// ProfileType names the kind of profile a payload contains.
type ProfileType string

const (
	ProfileTypeCPU       ProfileType = "cpu"
	ProfileTypeHeap      ProfileType = "heap"
	ProfileTypeGoroutine ProfileType = "goroutine"
	ProfileTypeMutex     ProfileType = "mutex"
	ProfileTypeBlock     ProfileType = "block"
)

// Profiler runs continuous CPU and heap profiling and exports the results as
// the OTLP profiles signal. Create it with New, start it once with Start and
// flush pending exports with Stop during shutdown.
type Profiler struct {
	cfg       Config
	res       *resource.Resource
	guard     *resourceGuard
	sanitizer *sanitizer
	inst      *instruments
	exporter  *exporter
	logger    *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// previous runtime mutex sampling rate, restored on Stop
	prevMutexFraction int
}

// New wires the profiler against the given resource (shared with the other
// telemetry signals) and meter provider (delivery-verification metrics).
func New(cfg Config, res *resource.Resource, meterProvider metric.MeterProvider, logger *slog.Logger) (*Profiler, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	if res == nil {
		res = resource.Default()
	}

	p := &Profiler{
		cfg:    cfg,
		res:    res,
		guard:  newResourceGuard(cfg.MaxProfilesPerMinute, cfg.MaxBufferBytes),
		logger: logger,
	}

	var err error
	if p.sanitizer, err = newSanitizer(cfg.RedactPatterns, cfg.RedactIPs); err != nil {
		return nil, err
	}

	queueLength := func() int64 {
		if p.exporter == nil {
			return 0
		}
		return p.exporter.queueLength()
	}
	if p.inst, err = newInstruments(meterProvider, queueLength, p.guard.bufferedBytes); err != nil {
		return nil, err
	}

	p.exporter = newExporter(cfg, p.guard, p.inst, logger)

	return p, nil
}

// Start launches the exporter and the CPU/heap collectors. The profiler runs
// until Stop is called; cancelling ctx also stops the collectors.
func (p *Profiler) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		return nil // already running
	}

	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.exporter.run()
	}()

	monitor := newCPUMonitor()

	collectors := []interface{ run(context.Context) }{
		newCPUCollector(p.cfg, monitor, p.guard, p.inst, p.process, p.logger),
		newMemoryCollector(p.cfg, monitor, p.guard, p.inst, p.process, p.logger),
		newLookupCollector("goroutine", ProfileTypeGoroutine, p.cfg.GoroutineInterval, p.cfg, monitor, p.guard, p.inst, p.process, p.logger, nil),
	}

	// mutex contention events are only sampled when the runtime rate is set;
	// the previous rate is restored on Stop
	if p.cfg.MutexFraction > 0 {
		p.prevMutexFraction = runtime.SetMutexProfileFraction(p.cfg.MutexFraction)
		collectors = append(collectors,
			newLookupCollector("mutex", ProfileTypeMutex, p.cfg.MutexInterval, p.cfg, monitor, p.guard, p.inst, p.process, p.logger, nil),
		)
	}

	// same for blocking events; SetBlockProfileRate has no getter, so Stop
	// restores the runtime default of 0 instead of the previous value
	if p.cfg.BlockRate > 0 {
		runtime.SetBlockProfileRate(p.cfg.BlockRate)
		collectors = append(collectors,
			newLookupCollector("block", ProfileTypeBlock, p.cfg.BlockInterval, p.cfg, monitor, p.guard, p.inst, p.process, p.logger, nil),
		)
	}

	for _, collector := range collectors {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			collector.run(ctx)
		}()
	}

	p.logger.Info("profiler: continuous profiling started",
		slog.String("endpoint", p.cfg.Endpoint),
		slog.Duration("cpu_interval", p.cfg.CPUInterval),
		slog.Duration("cpu_duration", p.cfg.CPUDuration),
		slog.Duration("memory_interval", p.cfg.MemoryInterval),
		slog.Duration("goroutine_interval", p.cfg.GoroutineInterval),
		slog.Duration("mutex_interval", p.cfg.MutexInterval),
		slog.Int("mutex_fraction", p.cfg.MutexFraction),
		slog.Duration("block_interval", p.cfg.BlockInterval),
		slog.Int("block_rate", p.cfg.BlockRate),
	)

	return nil
}

// Stop halts collection, flushes queued profiles and releases the metric
// callback. It returns early when ctx expires.
func (p *Profiler) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel == nil {
		return nil
	}
	p.cancel()
	p.cancel = nil

	if p.cfg.MutexFraction > 0 {
		runtime.SetMutexProfileFraction(p.prevMutexFraction)
	}
	if p.cfg.BlockRate > 0 {
		runtime.SetBlockProfileRate(0)
	}

	err := p.exporter.shutdown(ctx)

	collectorsDone := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(collectorsDone)
	}()
	select {
	case <-collectorsDone:
	case <-ctx.Done():
		err = ctx.Err()
	}

	if shutdownErr := p.inst.shutdown(); err == nil {
		err = shutdownErr
	}

	return err
}

// process is the sink shared by the collectors: it accounts the payload
// against the memory budget, parses and sanitizes it, converts it to OTLP
// and queues it for export. Each stage is bounded by the per-profile
// timeout.
func (p *Profiler) process(ctx context.Context, pt ProfileType, raw []byte) {
	if len(raw) == 0 {
		return
	}

	size := int64(len(raw))
	if err := p.guard.reserve(size); err != nil {
		p.inst.recordDropped(ctx, pt, reasonResourceLimit)
		return
	}

	deadline := time.Now().Add(p.cfg.ProfileTimeout)

	parsed, err := profile.ParseData(raw)
	if err != nil {
		p.guard.release(size)
		p.inst.recordDropped(ctx, pt, reasonEncodeError)
		p.logger.Warn("profiler: parsing profile", slog.String("type", string(pt)), slog.String("error", err.Error()))
		return
	}

	// security: redact before anything leaves the process
	p.sanitizer.sanitize(parsed)

	if time.Now().After(deadline) {
		p.guard.release(size)
		p.inst.recordDropped(ctx, pt, reasonTimeout)
		return
	}

	profiles, err := toOTLP(parsed, pt, p.res)
	if err != nil {
		p.guard.release(size)
		p.inst.recordDropped(ctx, pt, reasonEncodeError)
		p.logger.Warn("profiler: converting profile", slog.String("type", string(pt)), slog.String("error", err.Error()))
		return
	}

	if time.Now().After(deadline) {
		p.guard.release(size)
		p.inst.recordDropped(ctx, pt, reasonTimeout)
		return
	}

	env := envelope{
		profiles: profiles,
		records:  int64(profiles.ProfileCount()),
		rawBytes: size,
	}
	if !p.exporter.enqueue(env) {
		p.guard.release(size)
		p.inst.recordDropped(ctx, pt, reasonQueueFull)
	}
}
