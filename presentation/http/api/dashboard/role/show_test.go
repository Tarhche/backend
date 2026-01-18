package role

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getrole "github.com/khanzadimahdi/testproject/application/dashboard/role/getRole"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show role", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			u = user.User{UUID: "auth-user-uuid"}

			roleUUID = "role-uuid"
			a        = role.Role{
				UUID:        roleUUID,
				Name:        "Test Role",
				Description: "Test Description",
			}
		)

		roleRepository.On("GetOne", roleUUID).Return(a, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewShowHandler(getrole.NewUseCase(&roleRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", roleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-role-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			u = user.User{UUID: "auth-user-uuid"}

			roleUUID = "role-uuid"
		)

		roleRepository.On("GetOne", roleUUID).Return(role.Role{}, domain.ErrNotExists)
		defer roleRepository.AssertExpectations(t)

		handler := NewShowHandler(getrole.NewUseCase(&roleRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", roleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
