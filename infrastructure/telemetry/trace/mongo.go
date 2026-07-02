package trace

import (
	"context"
	"sync"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

// NewMongoCommandMonitor returns a mongo-driver event.CommandMonitor that
// starts a span for every command sent to the server and ends it once the
// reply arrives, recording failures as span errors. Wire it via
// options.Client().SetMonitor(...) so every repository gets DB spans without
// each of them needing to start one individually.
//
// Started/Succeeded/Failed for the same command are correlated by RequestID
// since the driver invokes them with independent contexts that don't carry
// the span started in Started back to the other two.
func NewMongoCommandMonitor(tracerName string) *event.CommandMonitor {
	tracer := otel.Tracer(tracerName)
	spans := &sync.Map{} // RequestID (int64) -> trace.Span

	return &event.CommandMonitor{
		Started: func(ctx context.Context, evt *event.CommandStartedEvent) {
			attrs := []attribute.KeyValue{
				semconv.DBSystemNameMongoDB,
				semconv.DBNamespace(evt.DatabaseName),
				semconv.DBOperationName(evt.CommandName),
			}

			if collection, ok := evt.Command.Lookup(evt.CommandName).StringValueOK(); ok {
				attrs = append(attrs, semconv.DBCollectionName(collection))
			}

			_, span := tracer.Start(ctx, "mongodb "+evt.CommandName,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(attrs...),
			)

			spans.Store(evt.RequestID, span)
		},
		Succeeded: func(_ context.Context, evt *event.CommandSucceededEvent) {
			if span, ok := spans.LoadAndDelete(evt.RequestID); ok {
				span.(trace.Span).End()
			}
		},
		Failed: func(_ context.Context, evt *event.CommandFailedEvent) {
			if span, ok := spans.LoadAndDelete(evt.RequestID); ok {
				_ = RecordError(span.(trace.Span), evt.Failure)
				span.(trace.Span).End()
			}
		},
	}
}
