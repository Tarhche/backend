package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/config/getConfig"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/user"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show config", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository

			u = user.User{UUID: "auth-user-uuid"}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}
		)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		defer configRepository.AssertExpectations(t)

		handler := NewShowHandler(getConfig.NewUseCase(&configRepository))

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
}
