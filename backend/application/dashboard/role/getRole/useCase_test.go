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
			elementRepository roles.MockRolesRepository

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

		elementRepository.On("GetOne", roleUUID).Return(a, nil)
		defer elementRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository).Execute(roleUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting a role fails", func(t *testing.T) {
		var (
			elementRepository roles.MockRolesRepository

			roleUUID      = "role-uuid"
			expectedError = errors.New("error")
		)

		elementRepository.On("GetOne", roleUUID).Once().Return(role.Role{}, expectedError)
		defer elementRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository).Execute(roleUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
