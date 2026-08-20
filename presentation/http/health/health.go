package health

import (
	"net/http"

	"go.opentelemetry.io/otel/trace"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

type healthHandler struct {
	useCase *checkhealth.UseCase
}

// Ensure healthHandler implements http.Handler.
var _ http.Handler = &healthHandler{}

func NewHealthHandler(useCase *checkhealth.UseCase) *healthHandler {
	return &healthHandler{
		useCase: useCase,
	}
}

// ServeHTTP reports whether the service can serve traffic, which means every
// dependency it needs answers. it is shared by all services and carries no
// openapi annotation on purpose: it lives outside the /api base path and exists
// for the container healthcheck (and therefore the rolling deploy), not for API
// consumers.
func (h *healthHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Add("Content-Type", "text/plain; charset=utf-8")

	if err := h.useCase.Execute(r.Context()); err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusServiceUnavailable)
		rw.Write([]byte(err.Error()))

		return
	}

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte("ok"))
}
