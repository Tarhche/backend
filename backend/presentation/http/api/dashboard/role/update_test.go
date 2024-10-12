package role

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
	updaterole "github.com/khanzadimahdi/testproject/application/dashboard/role/updateRole"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUpdateHandler(t *testing.T) {
	t.Run("update role", func(t *testing.T) {
		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			authorizer           domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = updaterole.Request{
				UUID:        "test-role-uuid",
				Name:        "test role name",
				Description: "test role description",
				Permissions: []string{"test1", "test2", "test3"},
				UserUUIDs:   []string{"user-uuid1", "user-uuid2"},
			}

			p = []permission.Permission{
				{Name: "permission name 1", Value: "test1"},
				{Name: "permission name 2", Value: "test2"},
				{Name: "permission name 3", Value: "test3"},
			}

			c = role.Role{
				UUID:        r.UUID,
				Name:        r.Name,
				Description: r.Description,
				Permissions: r.Permissions,
				UserUUIDs:   r.UserUUIDs,
			}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		roleRepository.On("Save", &c).Once().Return(c.UUID, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updaterole.NewUseCase(&roleRepository, &permissionRepository), &authorizer)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			authorizer           domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = updaterole.Request{
				UUID:        "test-role-uuid",
				Name:        "test role name",
				Description: "test role description",
				Permissions: []string{"test1", "test2", "test3"},
				UserUUIDs:   []string{"user-uuid1", "user-uuid2"},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateHandler(updaterole.NewUseCase(&roleRepository, &permissionRepository), &authorizer)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		permissionRepository.AssertNotCalled(t, "Get")
		roleRepository.AssertNotCalled(t, "Save")

		expectedBody, err := os.ReadFile("testdata/update-roles-validation-failed-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			authorizer           domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesUpdate).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateHandler(updaterole.NewUseCase(&roleRepository, &permissionRepository), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		permissionRepository.AssertNotCalled(t, "Get")
		roleRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			authorizer           domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.RolesUpdate).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateHandler(updaterole.NewUseCase(&roleRepository, &permissionRepository), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		permissionRepository.AssertNotCalled(t, "Get")
		roleRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
