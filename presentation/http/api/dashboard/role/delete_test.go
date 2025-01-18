package role

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleterole "github.com/khanzadimahdi/testproject/application/dashboard/role/deleteRole"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete role", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository
			authorizer     domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = deleterole.Request{RoleUUID: "role-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesDelete).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		roleRepository.On("Delete", r.RoleUUID).Return(nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deleterole.NewUseCase(&roleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.RoleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository
			authorizer     domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = deleterole.Request{RoleUUID: "role-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesDelete).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deleterole.NewUseCase(&roleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.RoleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		roleRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository
			authorizer     domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = deleterole.Request{RoleUUID: "role-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesDelete).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deleterole.NewUseCase(&roleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.RoleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		roleRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
