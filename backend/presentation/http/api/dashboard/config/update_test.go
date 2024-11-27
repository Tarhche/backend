package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/config/updateConfig"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateHandler(t *testing.T) {
	t.Parallel()

	t.Run("update config", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			r = updateConfig.Request{
				UserDefaultRoles: []string{"role1", "role2"},
			}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}

			savedConfig = config.Config{
				Revision:             loadedConfig.Revision,
				UserDefaultRoleUUIDs: r.UserDefaultRoles,
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		configRepository.On("Save", &savedConfig).Once().Return("new-revision-uuid", nil)
		defer configRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updateConfig.NewUseCase(&configRepository, &requestValidator), &authorizer)

		var payload bytes.Buffer
		json.NewEncoder(&payload).Encode(r)

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		requestValidator.On("Validate", &updateConfig.Request{}).Once().Return(domain.ValidationErrors{
			"user_default_roles": "user_default_roles is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewUpdateHandler(updateConfig.NewUseCase(&configRepository, &requestValidator), &authorizer)

		request := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/update-config-validation-errors-response.json")
		assert.NoError(t, err)

		configRepository.AssertNotCalled(t, "GetLatestRevision")
		configRepository.AssertNotCalled(t, "Save")

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			r = updateConfig.Request{
				UserDefaultRoles: []string{"role1", "role2"},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigUpdate).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateHandler(updateConfig.NewUseCase(&configRepository, &requestValidator), &authorizer)

		var payload bytes.Buffer
		json.NewEncoder(&payload).Encode(r)

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		configRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			r = updateConfig.Request{
				UserDefaultRoles: []string{"role1", "role2"},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ConfigUpdate).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateHandler(updateConfig.NewUseCase(&configRepository, &requestValidator), &authorizer)

		var payload bytes.Buffer
		json.NewEncoder(&payload).Encode(r)

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		configRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
