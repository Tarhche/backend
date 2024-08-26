package getrole

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("gets a role", func(t *testing.T) {
		var (
			roleRepository roles.MockRolesRepository

			roleUUID = "role-uuid"
			a        = role.Role{
				UUID:        roleUUID,
				Name:        "Test Role",
				Description: "Test Description",
			}
			expectedResponse = Response{
				UUID:        roleUUID,
				Name:        "Test Role",
				Description: "Test Description",
				Permissions: []string{},
				UserUUIDs:   []string{},
			}
		)

		roleRepository.On("GetOne", roleUUID).Return(a, nil)
		defer roleRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository).Execute(roleUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting a role fails", func(t *testing.T) {
		var (
			roleRepository roles.MockRolesRepository

			roleUUID      = "role-uuid"
			expectedError = errors.New("error")
		)

		roleRepository.On("GetOne", roleUUID).Once().Return(role.Role{}, expectedError)
		defer roleRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository).Execute(roleUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
