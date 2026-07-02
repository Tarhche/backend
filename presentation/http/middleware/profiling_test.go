package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
)

func TestProfilingMiddleware(t *testing.T) {
	t.Run("labels the request goroutine with the span context", func(t *testing.T) {
		provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
		t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

		var traceID string
		var found bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID, found = pprof.Label(r.Context(), "trace_id")
			w.WriteHeader(http.StatusOK)
		})

		m := NewProfilingMiddleware(next, profiler.NewTracedProfiler())

		// simulate the surrounding Telemetry middleware starting a span
		ctx, span := provider.Tracer("test").Start(context.Background(), "request")
		defer span.End()

		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		require.True(t, found)
		assert.Equal(t, span.SpanContext().TraceID().String(), traceID)
		assert.Equal(t, http.StatusOK, res.Code)
	})

	t.Run("passes the request through without a span", func(t *testing.T) {
		var handlerCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true

			assert.False(t, trace.SpanContextFromContext(r.Context()).IsValid())
			_, found := pprof.Label(r.Context(), "trace_id")
			assert.False(t, found)

			w.WriteHeader(http.StatusOK)
		})

		m := NewProfilingMiddleware(next, profiler.NewTracedProfiler())

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
		assert.Equal(t, http.StatusOK, res.Code)
	})
}
