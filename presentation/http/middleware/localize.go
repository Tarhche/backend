package middleware

import (
	"context"
	"net/http"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/localize"
)

// languageCodeHeader is the HTTP header that explicitly selects the request language.
const languageCodeHeader = "X-Language-Code"

// Scoper opens a scoped child container seeded with per-request values. It is
// satisfied by *provider.Manager.
type Scoper interface {
	Scope(ctx context.Context, opts ...provider.ScopeOption) (*provider.Scope, error)
}

// Localize resolves the request language, opens a request scope seeded with it
// and injects the language code and the scoped container into the request
// context before delegating to the next handler.
type Localize struct {
	next      http.Handler
	localizer *localize.Localizer
	scoper    Scoper
}

// Ensure Localize implements http.Handler.
var _ http.Handler = &Localize{}

// NewLocalizeMiddleware creates a new Localize middleware.
func NewLocalizeMiddleware(next http.Handler, localizer *localize.Localizer, scoper Scoper) *Localize {
	return &Localize{
		next:      next,
		localizer: localizer,
		scoper:    scoper,
	}
}

func (l *Localize) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	code := l.localizer.Resolve(r.Context(), r.Header.Get(languageCodeHeader))

	scope, err := l.scoper.Scope(r.Context(), provider.WithValue(localize.LanguageCode, code))
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer scope.Terminate(r.Context())

	ctx := localize.ToContext(r.Context(), code)
	ctx = containerToContext(ctx, scope.Container())

	l.next.ServeHTTP(rw, r.WithContext(ctx))
}

// containerKey and languageKey are unexported types used as keys for storing values in request contexts.
// Using unexported types prevents key collisions between packages using context values.
type containerKey struct{}

// ContainerFromContext returns the request-scoped container opened by the
// Localize middleware, or nil when none was set.
func ContainerFromContext(ctx context.Context) provider.Container {
	c, _ := ctx.Value(containerKey{}).(provider.Container)

	return c
}

func containerToContext(ctx context.Context, c provider.Container) context.Context {
	return context.WithValue(ctx, containerKey{}, c)
}

// ScopedHandler builds its handler from the request-scoped container that the
// Localize middleware stores in the context, so the handler (and the use case
// it builds) sees language-aware request-scoped services.
type ScopedHandler struct {
	build func(container provider.Container) http.Handler
}

var _ http.Handler = &ScopedHandler{}

func NewScopedHandler(build func(container provider.Container) http.Handler) *ScopedHandler {
	return &ScopedHandler{build: build}
}

func (h *ScopedHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.build(ContainerFromContext(r.Context())).ServeHTTP(rw, r)
}
