package trace

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func setGlobalTracerProvider(t *testing.T, tp *sdktrace.TracerProvider) {
	t.Helper()

	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })
}

func findCommand(t *testing.T, collection string) bson.Raw {
	t.Helper()

	raw, err := bson.Marshal(bson.D{{Key: "find", Value: collection}})
	require.NoError(t, err)

	return raw
}

func TestNewMongoCommandMonitor(t *testing.T) {
	t.Run("succeeded command produces an ended span without an error", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
		setGlobalTracerProvider(t, tp)

		monitor := NewMongoCommandMonitor("test-mongo")
		ctx := context.Background()

		monitor.Started(ctx, &event.CommandStartedEvent{
			Command:      findCommand(t, "users"),
			DatabaseName: "blog",
			CommandName:  "find",
			RequestID:    1,
		})
		monitor.Succeeded(ctx, &event.CommandSucceededEvent{
			CommandFinishedEvent: event.CommandFinishedEvent{
				CommandName: "find",
				RequestID:   1,
			},
		})

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		assert.Equal(t, "mongodb find", spans[0].Name)
		assert.Equal(t, codes.Unset, spans[0].Status.Code)
		assert.Contains(t, spans[0].Attributes, attribute.String("db.collection.name", "users"))
	})

	t.Run("failed command records the failure and ends the span", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
		setGlobalTracerProvider(t, tp)

		monitor := NewMongoCommandMonitor("test-mongo")
		ctx := context.Background()
		wantErr := errors.New("connection refused")

		monitor.Started(ctx, &event.CommandStartedEvent{
			Command:      findCommand(t, "users"),
			DatabaseName: "blog",
			CommandName:  "find",
			RequestID:    2,
		})
		monitor.Failed(ctx, &event.CommandFailedEvent{
			CommandFinishedEvent: event.CommandFinishedEvent{
				CommandName: "find",
				RequestID:   2,
			},
			Failure: wantErr,
		})

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		assert.Equal(t, codes.Error, spans[0].Status.Code)
		require.Len(t, spans[0].Events, 1)
		assert.Equal(t, "exception", spans[0].Events[0].Name)
	})

	t.Run("succeeded/failed for an unknown request id is a no-op", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
		setGlobalTracerProvider(t, tp)

		monitor := NewMongoCommandMonitor("test-mongo")
		ctx := context.Background()

		assert.NotPanics(t, func() {
			monitor.Succeeded(ctx, &event.CommandSucceededEvent{
				CommandFinishedEvent: event.CommandFinishedEvent{RequestID: 99},
			})
			monitor.Failed(ctx, &event.CommandFailedEvent{
				CommandFinishedEvent: event.CommandFinishedEvent{RequestID: 100},
				Failure:              errors.New("boom"),
			})
		})

		assert.Empty(t, exporter.GetSpans())
	})

	t.Run("administrative commands without a collection name don't set db.collection.name", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
		setGlobalTracerProvider(t, tp)

		raw, err := bson.Marshal(bson.D{{Key: "ping", Value: 1}})
		require.NoError(t, err)

		monitor := NewMongoCommandMonitor("test-mongo")
		ctx := context.Background()

		monitor.Started(ctx, &event.CommandStartedEvent{
			Command:      raw,
			DatabaseName: "blog",
			CommandName:  "ping",
			RequestID:    3,
		})
		monitor.Succeeded(ctx, &event.CommandSucceededEvent{
			CommandFinishedEvent: event.CommandFinishedEvent{CommandName: "ping", RequestID: 3},
		})

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		for _, attr := range spans[0].Attributes {
			assert.NotEqual(t, "db.collection.name", string(attr.Key))
		}
	})
}
