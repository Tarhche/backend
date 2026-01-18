package middleware

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

func TestAuthorizeMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("authorize middleware", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer domain.MockAuthorizer

			u = user.User{
				UUID: "user-test-uuid",
			}

			permission       = "test-permission"
			expectedResponse = "test response"
		)

		authorizer.On("Authorize", u.UUID, permission).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedResponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewAuthorizeMiddleware(next, &authorizer, permission)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Equal(t, expectedResponse, response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("authorize middleware with error", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer domain.MockAuthorizer

			u = user.User{
				UUID: "user-test-uuid",
			}

			permission       = "test-permission"
			expectedResponse = "test response"
		)

		authorizer.On("Authorize", u.UUID, permission).Once().Return(false, errors.New("some error"))
		defer authorizer.AssertExpectations(t)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedResponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewAuthorizeMiddleware(next, &authorizer, permission)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Empty(t, response.Body)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})

	t.Run("authorize middleware with forbidden", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer domain.MockAuthorizer

			u = user.User{
				UUID: "user-test-uuid",
			}

			permission       = "test-permission"
			expectedResponse = "test response"
		)

		authorizer.On("Authorize", u.UUID, permission).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedResponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewAuthorizeMiddleware(next, &authorizer, permission)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Empty(t, response.Body)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})
}
