package middleware

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type Authorize struct {
	next       http.Handler
	authorizer domain.Authorizer
	permission string
}

var _ http.Handler = &Authorize{}

func NewAuthorizeMiddleware(
	next http.Handler,
	authorizer domain.Authorizer,
	permission string,
) *Authorize {
	return &Authorize{
		next:       next,
		authorizer: authorizer,
		permission: permission,
	}
}

func (a *Authorize) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := a.authorizer.Authorize(r.Context(), userUUID, a.permission); err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusForbidden)
		return
	}

	a.next.ServeHTTP(rw, r)
}
