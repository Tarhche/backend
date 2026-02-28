package role

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	createrole "github.com/khanzadimahdi/testproject/application/dashboard/role/createRole"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("create role", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			requestValidator     validator.MockValidator
			translator           translator.TranslatorMock

			u = user.User{UUID: "auth-user-uuid"}

			r = createrole.Request{
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
				Name:        r.Name,
				Description: r.Description,
				Permissions: r.Permissions,
				UserUUIDs:   r.UserUUIDs,
			}

			roleUUID = "role-uuid"
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		roleRepository.On("Save", &c).Once().Return(roleUUID, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewCreateHandler(createrole.NewUseCase(&roleRepository, &permissionRepository, &requestValidator, &translator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		expectedBody, err := os.ReadFile("testdata/create-roles-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			requestValidator     validator.MockValidator
			translator           translator.TranslatorMock

			u = user.User{UUID: "auth-user-uuid"}
		)

		requestValidator.On("Validate", &createrole.Request{}).Once().Return(domain.ValidationErrors{
			"description": "description is required",
			"name":        "name is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewCreateHandler(createrole.NewUseCase(&roleRepository, &permissionRepository, &requestValidator, &translator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		permissionRepository.AssertNotCalled(t, "Get")
		roleRepository.AssertNotCalled(t, "Save")

		expectedBody, err := os.ReadFile("testdata/create-roles-validation-failed-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
