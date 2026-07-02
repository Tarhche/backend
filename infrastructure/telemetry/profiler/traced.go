package profiler

import (
	"context"
	"runtime/pprof"

	"go.opentelemetry.io/otel/trace"
)

// pprof label keys carrying the trace context. CPU samples taken while these
// labels are set are turned into OTLP profile links during conversion, which
// correlates flame graphs with individual traces.
const (
	traceIDLabelKey = "trace_id"
	spanIDLabelKey  = "span_id"
)

// TracedProfiler tags goroutines with the active span's identifiers so CPU
// profile samples can be correlated with traces. It is safe for concurrent
// use and cheap when no span is recording.
type TracedProfiler struct{}

func NewTracedProfiler() *TracedProfiler {
	return &TracedProfiler{}
}

// WithProfiling runs fn with the current span's trace_id/span_id attached as
// pprof goroutine labels. Labels propagate to goroutines started by fn and
// are restored afterwards. Without a valid span context fn runs unchanged.
func (tp *TracedProfiler) WithProfiling(ctx context.Context, fn func(context.Context)) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		fn(ctx)
		return
	}

	labels := pprof.Labels(
		traceIDLabelKey, spanContext.TraceID().String(),
		spanIDLabelKey, spanContext.SpanID().String(),
	)

	pprof.Do(ctx, labels, fn)
}
