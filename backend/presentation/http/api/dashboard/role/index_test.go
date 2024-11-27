package role

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getroles "github.com/khanzadimahdi/testproject/application/dashboard/role/getRoles"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("show roles", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			p = []role.Role{
				{
					UUID:        "test-uuid-01",
					Name:        "role-name-01",
					Description: "test role description-01",
					Permissions: []string{"permission-1"},
					UserUUIDs:   []string{"user-uuid-1"},
				},
				{
					UUID: "test-uuid-02",
					Name: "role-name-02",
				},
				{Name: "role-name-03"},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		rolesRepository.On("Count").Once().Return(uint(len(p)), nil)
		rolesRepository.On("GetAll", uint(0), uint(10)).Once().Return(p, nil)
		defer rolesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getroles.NewUseCase(&rolesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-roles-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		rolesRepository.On("Count").Once().Return(uint(0), nil)
		rolesRepository.On("GetAll", uint(0), uint(10)).Once().Return(nil, nil)
		defer rolesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getroles.NewUseCase(&rolesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-roles-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesIndex).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getroles.NewUseCase(&rolesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		rolesRepository.AssertNotCalled(t, "Count")
		rolesRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesIndex).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getroles.NewUseCase(&rolesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		rolesRepository.AssertNotCalled(t, "Count")
		rolesRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
