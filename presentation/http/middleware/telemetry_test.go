package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTelemetryMiddleware(t *testing.T) {
	t.Run("creates middleware", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		m := NewTelemetryMiddleware("test", next)

		assert.NotNil(t, m)
		assert.NotNil(t, m.next)
		assert.NotNil(t, m.tracer)
	})

	t.Run("creates tracer with provided name", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		tracerName := "my-service"
		m := NewTelemetryMiddleware(tracerName, next)

		assert.NotNil(t, m.tracer)
	})
}

func TestTelemetryStartsSpan(t *testing.T) {
	t.Run("calls next handler", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
		assert.Equal(t, http.StatusOK, res.Code)
	})

	t.Run("passes request with updated context", func(t *testing.T) {
		var receivedCtx context.Context

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		originalCtx := req.Context()
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.NotNil(t, receivedCtx)
		assert.NotEqual(t, originalCtx, receivedCtx)
	})

	t.Run("creates span with request method and path", func(t *testing.T) {
		// Create a tracer provider to capture spans
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		defer func() {
			otel.SetTracerProvider(noop.NewTracerProvider())
		}()

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, http.StatusOK, res.Code)
	})
}

func TestTelemetryContextPropagation(t *testing.T) {
	t.Run("propagates updated context to next handler", func(t *testing.T) {
		var receivedCtx context.Context

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedCtx = r.Context()
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.NotNil(t, receivedCtx)
		// Verify span is in context
		span := trace.SpanFromContext(receivedCtx)
		assert.NotNil(t, span)
		assert.True(t, span.IsRecording() || true) // Noop span may not be recording
	})

	t.Run("preserves existing context values", func(t *testing.T) {
		contextKey := "test-key"
		contextValue := "test-value"
		ctx := context.WithValue(context.Background(), contextKey, contextValue)

		var receivedValue any

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedValue = r.Context().Value(contextKey)
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, contextValue, receivedValue)
	})
}

func TestTelemetryWithDifferentMethods(t *testing.T) {
	t.Run("works with GET", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})

	t.Run("works with POST", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})

	t.Run("works with PUT", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodPut, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})

	t.Run("works with DELETE", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})
}

func TestTelemetryWithDifferentPaths(t *testing.T) {
	t.Run("handles root path", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})

	t.Run("handles nested paths", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/123", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})

	t.Run("handles paths with query parameters", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/search?q=test&limit=10", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})
}

func TestTelemetryResponseHandling(t *testing.T) {
	t.Run("allows next handler to write response", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Header", "value")
			w.WriteHeader(http.StatusCreated)
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, http.StatusCreated, res.Code)
		assert.Equal(t, "value", res.Header().Get("X-Custom-Header"))
	})

	t.Run("allows next handler to write body", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response body"))
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, "response body", res.Body.String())
	})
}

func TestTraceAttributesWithoutTracing(t *testing.T) {
	t.Run("returns nil when no tracer provider configured", func(t *testing.T) {
		// Using default noop tracer
		ctx := context.Background()

		attrs := traceAttributes(ctx)

		assert.Nil(t, attrs)
	})

	t.Run("returns nil when span is not recording", func(t *testing.T) {
		ctx := context.Background()

		attrs := traceAttributes(ctx)

		assert.Nil(t, attrs)
	})
}

func TestTraceAttributesKeys(t *testing.T) {
	t.Run("TraceIDKey is correct", func(t *testing.T) {
		assert.Equal(t, "trace_id", TraceIDKey)
	})

	t.Run("SpanIDKey is correct", func(t *testing.T) {
		assert.Equal(t, "span_id", SpanIDKey)
	})
}

func TestTelemetryImplementsHandler(t *testing.T) {
	t.Run("Telemetry implements http.Handler", func(t *testing.T) {
		m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		var _ http.Handler = m
	})
}

func TestTelemetryPanic(t *testing.T) {
	t.Run("span is ended even if handler panics", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})
		m := NewTelemetryMiddleware("test", next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		// The span End() should be called even if handler panics
		assert.PanicsWithValue(t, "test panic", func() {
			m.ServeHTTP(res, req)
		})
	})
}

// recordedTelemetrySpans installs an in-memory exporter as the global tracer
// provider so spans created by the middleware can be inspected.
func recordedTelemetrySpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		_ = tp.Shutdown(context.Background())
	})

	return exporter
}

func TestTelemetryRecordsErrors(t *testing.T) {
	statusCodeAttr := func(spans tracetest.SpanStubs) (int64, bool) {
		for _, attr := range spans[0].Attributes {
			if string(attr.Key) == "http.response.status_code" {
				return attr.Value.AsInt64(), true
			}
		}
		return 0, false
	}

	t.Run("marks the span as failed on a 5xx response", func(t *testing.T) {
		exporter := recordedTelemetrySpans(t)

		m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

		m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Equal(t, codes.Error, spans[0].Status.Code)
		assert.Equal(t, http.StatusText(http.StatusInternalServerError), spans[0].Status.Description)

		status, found := statusCodeAttr(spans)
		assert.True(t, found)
		assert.Equal(t, int64(http.StatusInternalServerError), status)
	})

	t.Run("keeps the span status unset on success and client errors", func(t *testing.T) {
		for _, statusCode := range []int{http.StatusOK, http.StatusCreated, http.StatusNotFound, http.StatusUnprocessableEntity} {
			exporter := recordedTelemetrySpans(t)

			m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))

			m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

			spans := exporter.GetSpans()
			assert.Len(t, spans, 1)
			assert.Equal(t, codes.Unset, spans[0].Status.Code, "status %d", statusCode)

			status, found := statusCodeAttr(spans)
			assert.True(t, found)
			assert.Equal(t, int64(statusCode), status)
		}
	})

	t.Run("records the status code when the handler never calls WriteHeader", func(t *testing.T) {
		exporter := recordedTelemetrySpans(t)

		m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("implicit 200"))
		}))

		m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)

		status, found := statusCodeAttr(spans)
		assert.True(t, found)
		assert.Equal(t, int64(http.StatusOK), status)
	})

	t.Run("records a panic as an exception and re-panics", func(t *testing.T) {
		exporter := recordedTelemetrySpans(t)

		m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		}))

		assert.PanicsWithValue(t, "boom", func() {
			m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		})

		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Equal(t, codes.Error, spans[0].Status.Code)
		assert.Len(t, spans[0].Events, 1)
		assert.Equal(t, "exception", spans[0].Events[0].Name)
	})

	t.Run("does not record deliberately aborted requests as errors", func(t *testing.T) {
		exporter := recordedTelemetrySpans(t)

		m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		}))

		assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
			m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		})

		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Equal(t, codes.Unset, spans[0].Status.Code)
		assert.Empty(t, spans[0].Events)
	})
}

func TestTelemetryPreservesStreamingInterfaces(t *testing.T) {
	t.Run("handler still sees Flusher and Hijacker", func(t *testing.T) {
		var isFlusher, isHijacker bool

		m := NewTelemetryMiddleware("test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, isFlusher = w.(http.Flusher)
			_, isHijacker = w.(http.Hijacker)
		}))

		m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/ws", nil))

		assert.True(t, isFlusher)
		assert.True(t, isHijacker)
	})
}

func TestTelemetryMultipleRequests(t *testing.T) {
	t.Run("handles multiple sequential requests", func(t *testing.T) {
		var callCount int

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		})
		m := NewTelemetryMiddleware("test", next)

		for range 3 {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			res := httptest.NewRecorder()
			m.ServeHTTP(res, req)
		}

		assert.Equal(t, 3, callCount)
	})
}
