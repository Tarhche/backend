package nats

import (
	"strings"

	"github.com/nats-io/nats.go"
)

// HeaderCarrier adapts a nats.Header to a propagation.TextMapCarrier.
// Unlike http.Header, nats.Header is case-sensitive and NATS does not
// preserve the MIME-canonical casing http.Header.Set would otherwise write,
// so keys are normalized to lowercase (matching the W3C tracecontext header
// names) on both read and write to make the round-trip case-insensitive.
type HeaderCarrier nats.Header

func (c HeaderCarrier) Get(key string) string {
	v := nats.Header(c).Values(strings.ToLower(key))
	if len(v) == 0 {
		return ""
	}

	return v[0]
}

func (c HeaderCarrier) Set(key, value string) {
	nats.Header(c).Set(strings.ToLower(key), value)
}

func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}

	return keys
}
