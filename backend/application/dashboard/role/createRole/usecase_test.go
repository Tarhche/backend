package createrole

import (
	"errors"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("creates a role", func(t *testing.T) {
		var (
			elementRepository    roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository

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

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		elementRepository.On("Save", &c).Once().Return(roleUUID, nil)
		defer elementRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository, &permissionRepository).Execute(r)
		assert.NoError(t, err)
		assert.Equal(t, &Response{UUID: roleUUID}, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			elementRepository    roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"name":        "name is required",
					"description": "description is required",
				},
			}
		)

		response, err := NewUseCase(&elementRepository, &permissionRepository).Execute(r)

		permissionRepository.AssertNotCalled(t, "Get")
		elementRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("at least one permission not exists", func(t *testing.T) {
		var (
			elementRepository    roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository

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
				ValidationErrors: validationErrors{
					"permissions": "one or more of permissions not exist",
				},
			}
		)

		permissionRepository.On("Get", r.Permissions).Once().Return(p, nil)
		defer permissionRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository, &permissionRepository).Execute(r)

		elementRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting permissions fails", func(t *testing.T) {
		var (
			elementRepository    roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository

			r = Request{
				Name:        "test role name",
				Description: "test role description",
				Permissions: []string{"test1", "test2", "test3"},
				UserUUIDs:   []string{"user-uuid1", "user-uuid2"},
			}

			expectedErr = errors.New("error happened")
		)

		permissionRepository.On("Get", r.Permissions).Once().Return(nil, expectedErr)
		defer permissionRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository, &permissionRepository).Execute(r)

		elementRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving the role fails", func(t *testing.T) {
		var (
			elementRepository    roles.MockRolesRepository
			permissionRepository permissions.MockPermissionsRepository

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

		elementRepository.On("Save", &c).Once().Return("", expectedErr)
		defer elementRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository, &permissionRepository).Execute(r)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
