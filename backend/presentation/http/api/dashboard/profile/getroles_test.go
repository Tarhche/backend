package profile

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/getRoles"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestGetRolesHandler(t *testing.T) {
	t.Parallel()

	t.Run("get roles", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			u = user.User{
				UUID: "test-user-uuid",
			}

			rl = []role.Role{
				{
					UUID:        "role-uuid-1",
					Name:        "role-1",
					Description: "role description 1",
					Permissions: []string{"permission-1", "permission-2"},
					UserUUIDs:   []string{"test-user-uuid-1", "test-user-uuid-2"},
				},
				{
					UUID:        "role-uuid-2",
					Name:        "role-2",
					Description: "role description 2",
					Permissions: []string{"permission-1", "permission-5"},
					UserUUIDs:   []string{"test-user-uuid-2"},
				},
				{
					UUID:        "role-uuid-3",
					Name:        "role-3",
					Description: "role description 3",
				},
			}
		)

		roleRepository.On("GetByUserUUID", u.UUID).Once().Return(rl, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewGetRolesHandler(getRoles.NewUseCase(&roleRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/get-roles-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			u = user.User{
				UUID: "test-user-uuid",
			}
		)

		roleRepository.On("GetByUserUUID", u.UUID).Once().Return(nil, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewGetRolesHandler(getRoles.NewUseCase(&roleRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/get-roles-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			u = user.User{
				UUID: "test-user-uuid",
			}
		)

		roleRepository.On("GetByUserUUID", u.UUID).Once().Return(nil, errors.New("unexpected error"))
		defer roleRepository.AssertExpectations(t)

		handler := NewGetRolesHandler(getRoles.NewUseCase(&roleRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
