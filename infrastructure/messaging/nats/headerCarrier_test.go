package nats

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Ensure HeaderCarrier implements the propagation.TextMapCarrier interface.
var _ propagation.TextMapCarrier = HeaderCarrier{}

func TestHeaderCarrierGet(t *testing.T) {
	t.Run("returns empty string for a missing key", func(t *testing.T) {
		c := HeaderCarrier(nats.Header{})

		assert.Empty(t, c.Get("traceparent"))
	})

	t.Run("finds a lowercase key regardless of the queried casing", func(t *testing.T) {
		c := HeaderCarrier(nats.Header{"traceparent": {"value"}})

		assert.Equal(t, "value", c.Get("traceparent"))
		assert.Equal(t, "value", c.Get("Traceparent"))
		assert.Equal(t, "value", c.Get("TRACEPARENT"))
	})

	t.Run("returns the first value when the key holds several", func(t *testing.T) {
		c := HeaderCarrier(nats.Header{"tracestate": {"first", "second"}})

		assert.Equal(t, "first", c.Get("tracestate"))
	})

	t.Run("only lowercase-normalized keys are visible", func(t *testing.T) {
		// nats.Header is case-sensitive; the carrier normalizes to lowercase
		// on both read and write, so keys stored with other casings are
		// intentionally invisible
		c := HeaderCarrier(nats.Header{"Traceparent": {"value"}})

		assert.Empty(t, c.Get("Traceparent"))
	})
}

func TestHeaderCarrierSet(t *testing.T) {
	t.Run("stores the key lowercased", func(t *testing.T) {
		header := nats.Header{}
		c := HeaderCarrier(header)

		c.Set("Traceparent", "value")

		assert.Equal(t, []string{"value"}, header["traceparent"])
		assert.NotContains(t, header, "Traceparent")
	})

	t.Run("replaces an existing value", func(t *testing.T) {
		header := nats.Header{"traceparent": {"old"}}
		c := HeaderCarrier(header)

		c.Set("traceparent", "new")

		assert.Equal(t, []string{"new"}, header["traceparent"])
	})
}

func TestHeaderCarrierKeys(t *testing.T) {
	t.Run("returns no keys for an empty header", func(t *testing.T) {
		c := HeaderCarrier(nats.Header{})

		assert.Empty(t, c.Keys())
	})

	t.Run("returns all keys", func(t *testing.T) {
		c := HeaderCarrier(nats.Header{
			"traceparent": {"value"},
			"tracestate":  {"value"},
		})

		assert.ElementsMatch(t, []string{"traceparent", "tracestate"}, c.Keys())
	})
}

func TestHeaderCarrierPropagation(t *testing.T) {
	// example identifiers from the W3C trace context specification
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	assert.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	assert.NoError(t, err)

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	propagator := propagation.TraceContext{}
	header := nats.Header{}

	propagator.Inject(trace.ContextWithSpanContext(context.Background(), spanContext), HeaderCarrier(header))

	t.Run("injects the W3C-conventional lowercase key on the wire", func(t *testing.T) {
		assert.Equal(t,
			[]string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
			header["traceparent"],
		)
	})

	t.Run("extract restores the injected span context as remote", func(t *testing.T) {
		extracted := trace.SpanContextFromContext(
			propagator.Extract(context.Background(), HeaderCarrier(header)),
		)

		assert.True(t, extracted.IsValid())
		assert.True(t, extracted.IsRemote())
		assert.Equal(t, traceID, extracted.TraceID())
		assert.Equal(t, spanID, extracted.SpanID())
		assert.True(t, extracted.IsSampled())
	})
}
