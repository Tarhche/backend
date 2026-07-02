// Package trace provides small helpers shared by infrastructure adapters that
// start their own spans around I/O or CPU-heavy work which isn't already
// covered by the per-HTTP-request span (e.g. background jobs, DB drivers).
package trace

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RecordError marks span as failed and attaches err to it when err is not
// nil. It returns err unchanged so call sites can wrap a call in one line:
//
//	return trace.RecordError(span, err)
func RecordError(span trace.Span, err error) error {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}
