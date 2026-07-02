package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	infraHttp "github.com/khanzadimahdi/testproject/infrastructure/http"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

const (
	// TraceIDKey is the attribute key used when the trace identifier is logged.
	TraceIDKey = "trace_id"

	// SpanIDKey is the attribute key used when the span identifier is logged.
	SpanIDKey = "span_id"
)

// Telemetry starts an OpenTelemetry span for each incoming request so that
// downstream handlers and middleware (such as logging) can correlate work with
// a trace. Failures are recorded on the span: 5xx responses mark it with an
// error status and panics are attached as exception events before they
// propagate to the recovery middleware. When no TracerProvider is configured
// the global no-op tracer is used and the middleware simply passes the
// request through.
type Telemetry struct {
	next   http.Handler
	tracer trace.Tracer
}

// Ensure Telemetry implements the http.Handler interface.
var _ http.Handler = &Telemetry{}

func NewTelemetryMiddleware(tracerName string, next http.Handler) *Telemetry {
	return &Telemetry{
		next:   next,
		tracer: otel.Tracer(tracerName),
	}
}

func (m *Telemetry) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx, span := m.tracer.Start(r.Context(), r.Method+" "+r.URL.Path)

	// the wrapper records the status code the handler chain writes; it keeps
	// Hijacker/Flusher support, so streaming and websocket routes still work
	wrapped := infraHttp.NewResponseWriter(rw, 0, false)

	// span.End is called inside the closure rather than deferred directly:
	// the SDK's End records any in-flight panic as an exception event on its
	// own, which would duplicate the one recorded here
	defer func() {
		if v := recover(); v != nil {
			// record the failure on the span, then let the recovery
			// middleware produce the 500 response and log the stack;
			// http.ErrAbortHandler is the sentinel for deliberately aborted
			// requests and is not an application error
			if v != http.ErrAbortHandler {
				_ = infraTrace.RecordError(span, fmt.Errorf("panic: %v", v))
			}
			span.End()
			panic(v)
		}

		status := wrapped.Status()
		span.SetAttributes(semconv.HTTPResponseStatusCode(status))

		// only server-side failures flag a server span as failed; 4xx are
		// client errors and keep the status unset per OpenTelemetry semantics
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		span.End()
	}()

	m.next.ServeHTTP(wrapped, r.WithContext(ctx))
}

// traceAttributes returns the trace and span identifiers of the span carried by
// ctx. It returns nil when the span is not recording, e.g. when no
// TracerProvider is configured.
func traceAttributes(ctx context.Context) []slog.Attr {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	spanCtx := span.SpanContext()
	attrs := make([]slog.Attr, 0, 2)

	if spanCtx.HasTraceID() {
		attrs = append(attrs, slog.String(TraceIDKey, spanCtx.TraceID().String()))
	}

	if spanCtx.HasSpanID() {
		attrs = append(attrs, slog.String(SpanIDKey, spanCtx.SpanID().String()))
	}

	return attrs
}
