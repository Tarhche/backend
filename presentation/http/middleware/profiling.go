package middleware

import (
	"context"
	"net/http"

	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
)

// Profiling tags the goroutine handling each request with the active span's
// trace_id/span_id pprof labels, so CPU profile samples collected by the
// continuous profiler can be correlated with individual traces. It must sit
// inside the Telemetry middleware, which starts the span.
type Profiling struct {
	next     http.Handler
	profiler *profiler.TracedProfiler
}

// Ensure Profiling implements the http.Handler interface.
var _ http.Handler = &Profiling{}

func NewProfilingMiddleware(next http.Handler, profiler *profiler.TracedProfiler) *Profiling {
	return &Profiling{
		next:     next,
		profiler: profiler,
	}
}

func (m *Profiling) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	m.profiler.WithProfiling(r.Context(), func(ctx context.Context) {
		m.next.ServeHTTP(rw, r.WithContext(ctx))
	})
}
