package permission

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getpermissions "github.com/khanzadimahdi/testproject/application/dashboard/permission/getPermissions"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("permissions", func(t *testing.T) {
		t.Parallel()

		var (
			permissionsRepository permissions.MockPermissionsRepository
			authorizer            domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			p = []permission.Permission{
				{Name: "permission-name-01", Value: "permission-value-01"},
				{Name: "permission-name-02", Value: "permission-value-02"},
				{Name: "permission-name-03", Value: "permission-value-03"},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.PermissionsIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		permissionsRepository.On("GetAll").Once().Return(p)
		defer permissionsRepository.AssertExpectations(t)

		handler := NewIndexHandler(getpermissions.NewUseCase(&permissionsRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-permissions-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			permissionsRepository permissions.MockPermissionsRepository
			authorizer            domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.PermissionsIndex).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getpermissions.NewUseCase(&permissionsRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		permissionsRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			permissionsRepository permissions.MockPermissionsRepository
			authorizer            domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.PermissionsIndex).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getpermissions.NewUseCase(&permissionsRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		permissionsRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
