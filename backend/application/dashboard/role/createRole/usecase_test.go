package createrole

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/role"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeOf(func(*translatorContract.Params) {}).Name()

	t.Run("creates a role", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			validator            validator.MockValidator
			translator           translator.TranslatorMock

			r = Request{
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

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		roleRepository.On("Save", &c).Once().Return(roleUUID, nil)
		defer roleRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository, &permissionRepository, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.Equal(t, &Response{UUID: roleUUID}, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			validator            validator.MockValidator
			translator           translator.TranslatorMock

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"name":        "name is required",
					"description": "description is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository, &permissionRepository, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		permissionRepository.AssertNotCalled(t, "Get")
		roleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("at least one permission not exists", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			validator            validator.MockValidator
			translator           translator.TranslatorMock

			r = Request{
				Name:        "test role name",
				Description: "test role description",
				Permissions: []string{"test1", "test2", "test3"},
				UserUUIDs:   []string{"user-uuid1", "user-uuid2"},
			}

			p = []permission.Permission{
				{Name: "permission name 1", Value: "test1"},
				{Name: "permission name 2", Value: "test2"},
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"permissions": "one or more of permissions not exist",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["permissions"],
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["permissions"])
		defer translator.AssertExpectations(t)

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository, &permissionRepository, &validator, &translator).Execute(&r)

		roleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting permissions fails", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			validator            validator.MockValidator
			translator           translator.TranslatorMock

			r = Request{
				Name:        "test role name",
				Description: "test role description",
				Permissions: []string{"test1", "test2", "test3"},
				UserUUIDs:   []string{"user-uuid1", "user-uuid2"},
			}

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		permissionRepository.On("Get", r.Permissions).Once().Return(nil, expectedErr)
		defer permissionRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository, &permissionRepository, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		roleRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving the role fails", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository       roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository
			validator            validator.MockValidator
			translator           translator.TranslatorMock

			r = Request{
				Name:        "test role name",
				Description: "test role description",
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

			expectedErr = errors.New("error happened")
		)

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		roleRepository.On("Save", &c).Once().Return("", expectedErr)
		defer roleRepository.AssertExpectations(t)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository, &permissionRepository, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
