package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestRequires(t *testing.T) {
	t.Parallel()

	handler := http.NotFoundHandler()

	var userRepository users.MockUsersRepository
	var authorizer domain.MockAuthorizer

	t.Run("a route anybody may call asks for nothing", func(t *testing.T) {
		requirements := Requires(handler)

		assert.True(t, requirements.Public())
		assert.False(t, requirements.Authenticated)
		assert.Empty(t, requirements.Permission)
	})

	t.Run("a route behind a cache is still the route it wraps", func(t *testing.T) {
		requirements := Requires(NewCacheMiddleware(
			NewAuthenticateMiddleware(
				NewAuthorizeMiddleware(handler, &authorizer, permission.ArticlesIndex),
				&jwt.JWT{},
				&userRepository,
			),
			nil,
		))

		assert.Equal(t, permission.ArticlesIndex, requirements.Permission)
		assert.True(t, requirements.Authenticated)
	})

	t.Run("a route only a signed-in caller may reach says so", func(t *testing.T) {
		requirements := Requires(NewAuthenticateMiddleware(handler, &jwt.JWT{}, &userRepository))

		assert.True(t, requirements.Authenticated)
		assert.Empty(t, requirements.Permission)
		assert.False(t, requirements.Public())
	})

	t.Run("a route served under a permission says which", func(t *testing.T) {
		requirements := Requires(NewAuthenticateMiddleware(
			NewAuthorizeMiddleware(handler, &authorizer, permission.RunnerTasksCreate),
			&jwt.JWT{},
			&userRepository,
		))

		assert.True(t, requirements.Authenticated)
		assert.Equal(t, permission.RunnerTasksCreate, requirements.Permission)
	})
}
