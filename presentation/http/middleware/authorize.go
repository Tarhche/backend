package middleware

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// Authorize lets a request through when the person making it holds any of the
// permissions the route was given.
//
// Two permissions often cover the same action — one over everybody's things and
// one over one's own — and a route that takes both is admitting somebody who
// may still only act on some of what lies behind it. Which of the two they hold
// is what the handler then has to make the difference on.
type Authorize struct {
	next        http.Handler
	authorizer  domain.Authorizer
	permissions []string
}

var _ http.Handler = &Authorize{}

func NewAuthorizeMiddleware(
	next http.Handler,
	authorizer domain.Authorizer,
	permissions ...string,
) *Authorize {
	return &Authorize{
		next:        next,
		authorizer:  authorizer,
		permissions: permissions,
	}
}

func (a *Authorize) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	for _, permission := range a.permissions {
		ok, err := a.authorizer.Authorize(r.Context(), userUUID, permission)
		if err != nil {
			infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
			rw.WriteHeader(http.StatusInternalServerError)

			return
		}

		if ok {
			a.next.ServeHTTP(rw, r)

			return
		}
	}

	rw.WriteHeader(http.StatusForbidden)
}
