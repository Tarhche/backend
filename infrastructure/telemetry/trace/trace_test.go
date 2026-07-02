package trace

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newRecordedSpan(t *testing.T) (*tracetest.InMemoryExporter, trace.Span) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "span")

	return exporter, span
}

func TestRecordError(t *testing.T) {
	t.Run("returns nil and leaves span unmarked when err is nil", func(t *testing.T) {
		exporter, span := newRecordedSpan(t)

		err := RecordError(span, nil)
		span.End()

		assert.NoError(t, err)

		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Empty(t, spans[0].Events)
		assert.Equal(t, codes.Unset, spans[0].Status.Code)
	})

	t.Run("records the error and sets the span status when err is not nil", func(t *testing.T) {
		exporter, span := newRecordedSpan(t)

		wantErr := errors.New("boom")
		err := RecordError(span, wantErr)
		span.End()

		assert.Equal(t, wantErr, err)

		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Len(t, spans[0].Events, 1)
		assert.Equal(t, "exception", spans[0].Events[0].Name)
		assert.Equal(t, codes.Error, spans[0].Status.Code)
		assert.Equal(t, wantErr.Error(), spans[0].Status.Description)
	})
}
