package profiler

import (
	"context"
	"runtime/pprof"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracedProfilerWithProfiling(t *testing.T) {
	tp := NewTracedProfiler()

	t.Run("attaches the span context as pprof labels", func(t *testing.T) {
		provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
		t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

		ctx, span := provider.Tracer("test").Start(t.Context(), "span")
		defer span.End()

		var called bool
		tp.WithProfiling(ctx, func(ctx context.Context) {
			called = true

			traceID, ok := pprof.Label(ctx, traceIDLabelKey)
			require.True(t, ok)
			assert.Equal(t, span.SpanContext().TraceID().String(), traceID)

			spanID, ok := pprof.Label(ctx, spanIDLabelKey)
			require.True(t, ok)
			assert.Equal(t, span.SpanContext().SpanID().String(), spanID)
		})

		assert.True(t, called)
	})

	t.Run("runs the function unchanged without a span", func(t *testing.T) {
		var called bool
		tp.WithProfiling(t.Context(), func(ctx context.Context) {
			called = true

			_, ok := pprof.Label(ctx, traceIDLabelKey)
			assert.False(t, ok)
		})

		assert.True(t, called)
	})
}
