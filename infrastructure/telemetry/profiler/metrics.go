package profiler

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// scopeName identifies this instrumentation library on exported signals.
const scopeName = "github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"

// attribute keys used on the profiler's own metrics.
const (
	profileTypeKey = "profile.type"
	dropReasonKey  = "reason"
)

// drop/failure reasons recorded on profiler.profiles.dropped and
// profiler.profiles.failed.
const (
	reasonAdaptive      = "adaptive_sampling"
	reasonResourceLimit = "resource_limit"
	reasonContention    = "profiler_contention"
	reasonTimeout       = "timeout"
	reasonEncodeError   = "encode_error"
	reasonQueueFull     = "queue_full"
	reasonRejected      = "rejected"
	reasonPermanent     = "permanent_error"
	reasonExhausted     = "retries_exhausted"
)

// instruments bundles the delivery-verification metrics: everything needed
// to answer "are profiles being produced, and do they actually arrive?".
type instruments struct {
	collected          metric.Int64Counter
	dropped            metric.Int64Counter
	sent               metric.Int64Counter
	delivered          metric.Int64Counter
	failed             metric.Int64Counter
	retries            metric.Int64Counter
	exportDuration     metric.Float64Histogram
	payloadSize        metric.Int64Histogram
	collectionDuration metric.Float64Histogram
	gcPause            metric.Float64Histogram

	// state read by the observable gauges
	sentTotal           atomic.Int64
	deliveredTotal      atomic.Int64
	failedTotal         atomic.Int64
	cpuIntervalMs       atomic.Int64
	memIntervalMs       atomic.Int64
	goroutineIntervalMs atomic.Int64
	mutexIntervalMs     atomic.Int64
	blockIntervalMs     atomic.Int64

	registration metric.Registration
}

// newInstruments creates all instruments on the given meter provider.
// queueLength and bufferedBytes are sampled on every metrics collection.
func newInstruments(provider metric.MeterProvider, queueLength func() int64, bufferedBytes func() int64) (*instruments, error) {
	meter := provider.Meter(scopeName)
	inst := &instruments{}

	var err, errs error

	if inst.collected, err = meter.Int64Counter(
		"profiler.profiles.collected",
		metric.WithDescription("Profiles captured from the runtime, by profile type."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.dropped, err = meter.Int64Counter(
		"profiler.profiles.dropped",
		metric.WithDescription("Profiles skipped or discarded before export, by reason."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.sent, err = meter.Int64Counter(
		"profiler.profiles.sent",
		metric.WithDescription("OTLP profile records handed to the exporter transport."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.delivered, err = meter.Int64Counter(
		"profiler.profiles.delivered",
		metric.WithDescription("OTLP profile records confirmed by the backend."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.failed, err = meter.Int64Counter(
		"profiler.profiles.failed",
		metric.WithDescription("OTLP profile records that could not be delivered, by reason."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.retries, err = meter.Int64Counter(
		"profiler.export.retries",
		metric.WithDescription("Export attempts retried after a retryable failure."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.exportDuration, err = meter.Float64Histogram(
		"profiler.export.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Wall time of a full export (including retries)."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.payloadSize, err = meter.Int64Histogram(
		"profiler.export.payload.size",
		metric.WithUnit("By"),
		metric.WithDescription("Compressed OTLP payload size per export request."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.collectionDuration, err = meter.Float64Histogram(
		"profiler.collection.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Time from starting a collection until it was queued for export."),
	); err != nil {
		errs = errors.Join(errs, err)
	}
	if inst.gcPause, err = meter.Float64Histogram(
		"runtime.gc.pause_duration_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Stop-the-world garbage collection pause durations."),
	); err != nil {
		errs = errors.Join(errs, err)
	}

	successRatio, err := meter.Float64ObservableGauge(
		"profiler.delivery.success_ratio",
		metric.WithDescription("Delivered/sent profile records since start (1 when nothing was sent yet)."),
	)
	errs = errors.Join(errs, err)
	pending, err := meter.Int64ObservableGauge(
		"profiler.export.pending",
		metric.WithDescription("Profile records sent but neither confirmed nor failed yet."),
	)
	errs = errors.Join(errs, err)
	queueLen, err := meter.Int64ObservableGauge(
		"profiler.export.queue.length",
		metric.WithDescription("Collections waiting in the export queue."),
	)
	errs = errors.Join(errs, err)
	buffered, err := meter.Int64ObservableGauge(
		"profiler.buffer.bytes",
		metric.WithUnit("By"),
		metric.WithDescription("Memory held by in-flight profile payloads."),
	)
	errs = errors.Join(errs, err)
	interval, err := meter.Float64ObservableGauge(
		"profiler.collection.interval",
		metric.WithUnit("s"),
		metric.WithDescription("Current adaptive collection interval, by profile type."),
	)
	errs = errors.Join(errs, err)
	heapAlloc, err := meter.Int64ObservableGauge(
		"runtime.memory.heap_alloc_bytes",
		metric.WithUnit("By"),
		metric.WithDescription("Bytes of allocated heap objects."),
	)
	errs = errors.Join(errs, err)
	heapObjects, err := meter.Int64ObservableGauge(
		"runtime.memory.heap_objects",
		metric.WithDescription("Number of allocated heap objects."),
	)
	errs = errors.Join(errs, err)

	if errs != nil {
		return nil, errs
	}

	intervals := []struct {
		pt    ProfileType
		value *atomic.Int64
	}{
		{ProfileTypeCPU, &inst.cpuIntervalMs},
		{ProfileTypeHeap, &inst.memIntervalMs},
		{ProfileTypeGoroutine, &inst.goroutineIntervalMs},
		{ProfileTypeMutex, &inst.mutexIntervalMs},
		{ProfileTypeBlock, &inst.blockIntervalMs},
	}

	inst.registration, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		sent := inst.sentTotal.Load()
		delivered := inst.deliveredTotal.Load()
		failed := inst.failedTotal.Load()

		ratio := 1.0
		if sent > 0 {
			ratio = float64(delivered) / float64(sent)
		}
		o.ObserveFloat64(successRatio, ratio)
		o.ObserveInt64(pending, max(sent-delivered-failed, 0))
		o.ObserveInt64(queueLen, queueLength())
		o.ObserveInt64(buffered, bufferedBytes())
		for _, i := range intervals {
			// intervals stay unreported (0) until their collector runs
			if ms := i.value.Load(); ms > 0 {
				o.ObserveFloat64(interval, float64(ms)/1e3, metric.WithAttributes(attribute.String(profileTypeKey, string(i.pt))))
			}
		}

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		o.ObserveInt64(heapAlloc, int64(ms.HeapAlloc))
		o.ObserveInt64(heapObjects, int64(ms.HeapObjects))

		return nil
	}, successRatio, pending, queueLen, buffered, interval, heapAlloc, heapObjects)
	if err != nil {
		return nil, err
	}

	return inst, nil
}

func (inst *instruments) shutdown() error {
	if inst.registration == nil {
		return nil
	}

	return inst.registration.Unregister()
}

func (inst *instruments) recordCollected(ctx context.Context, pt ProfileType) {
	inst.collected.Add(ctx, 1, metric.WithAttributes(attribute.String(profileTypeKey, string(pt))))
}

func (inst *instruments) recordDropped(ctx context.Context, pt ProfileType, reason string) {
	inst.dropped.Add(ctx, 1, metric.WithAttributes(
		attribute.String(profileTypeKey, string(pt)),
		attribute.String(dropReasonKey, reason),
	))
}

func (inst *instruments) recordSent(ctx context.Context, records int64) {
	inst.sent.Add(ctx, records)
	inst.sentTotal.Add(records)
}

func (inst *instruments) recordDelivered(ctx context.Context, records int64) {
	inst.delivered.Add(ctx, records)
	inst.deliveredTotal.Add(records)
}

func (inst *instruments) recordFailed(ctx context.Context, records int64, reason string) {
	inst.failed.Add(ctx, records, metric.WithAttributes(attribute.String(dropReasonKey, reason)))
	inst.failedTotal.Add(records)
}

func (inst *instruments) recordExport(ctx context.Context, duration time.Duration, payloadBytes int) {
	inst.exportDuration.Record(ctx, duration.Seconds())
	inst.payloadSize.Record(ctx, int64(payloadBytes))
}

func (inst *instruments) recordCollection(ctx context.Context, pt ProfileType, duration time.Duration) {
	inst.collectionDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attribute.String(profileTypeKey, string(pt))))
}

func (inst *instruments) recordGCPause(ctx context.Context, pause time.Duration) {
	inst.gcPause.Record(ctx, pause.Seconds())
}

func (inst *instruments) setInterval(pt ProfileType, interval time.Duration) {
	switch pt {
	case ProfileTypeCPU:
		inst.cpuIntervalMs.Store(interval.Milliseconds())
	case ProfileTypeHeap:
		inst.memIntervalMs.Store(interval.Milliseconds())
	case ProfileTypeGoroutine:
		inst.goroutineIntervalMs.Store(interval.Milliseconds())
	case ProfileTypeMutex:
		inst.mutexIntervalMs.Store(interval.Milliseconds())
	case ProfileTypeBlock:
		inst.blockIntervalMs.Store(interval.Milliseconds())
	}
}
