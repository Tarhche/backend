package config

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/config/getConfig"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
)

func TestShowHandler(t *testing.T) {
	t.Run("show config", func(t *testing.T) {
		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigShow).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		defer configRepository.AssertExpectations(t)

		handler := NewShowHandler(getConfig.NewUseCase(&configRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-config-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigShow).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewShowHandler(getConfig.NewUseCase(&configRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		configRepository.AssertNotCalled(t, "GetLatestRevision")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigShow).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewShowHandler(getConfig.NewUseCase(&configRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		configRepository.AssertNotCalled(t, "GetLatestRevision")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
