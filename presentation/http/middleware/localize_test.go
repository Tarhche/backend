package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danceable/container/resolve"
	"github.com/danceable/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/application/localize"
)

func TestLocalize_ServeHTTP(t *testing.T) {
	t.Parallel()

	var resolverMock resolver.MockResolver
	localizer := localize.New(&resolverMock)

	var (
		seededCode   string
		contextCode  string
		gotContainer provider.Container
	)

	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotContainer = ContainerFromContext(r.Context())
		require.NotNil(t, gotContainer)
		require.NoError(t, gotContainer.Resolve(&seededCode, resolve.WithName(localize.LanguageCode)))
		contextCode = localize.FromContext(r.Context())
		rw.WriteHeader(http.StatusOK)
	})

	handler := NewLocalizeMiddleware(next, localizer, provider.Default)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(languageCodeHeader, "en")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "en", seededCode)
	assert.Equal(t, "en", contextCode)
}

func TestScopedHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	scope, err := provider.Default.Scope(context.Background())
	require.NoError(t, err)
	defer scope.Terminate(context.Background())

	var got provider.Container
	handler := NewScopedHandler(func(c provider.Container) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			got = c
			rw.WriteHeader(http.StatusOK)
		})
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(containerToContext(r.Context(), scope.Container()))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Same(t, scope.Container(), got)
}
