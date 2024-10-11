package getRoles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("get user's roles", func(t *testing.T) {
		var (
			roleRepository roles.MockRolesRepository

			userUUID = "test-user-uuid"

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

			expectedResponse = Response{
				Items: []roleResponse{
					{
						UUID:        rl[0].UUID,
						Name:        rl[0].Name,
						Description: rl[0].Description,
						Permissions: rl[0].Permissions,
					},
					{
						UUID:        rl[1].UUID,
						Name:        rl[1].Name,
						Description: rl[1].Description,
						Permissions: rl[1].Permissions,
					},
					{
						UUID:        rl[2].UUID,
						Name:        rl[2].Name,
						Description: rl[2].Description,
						Permissions: []string{},
					},
				},
			}
		)

		roleRepository.On("GetByUserUUID", userUUID).Once().Return(rl, nil)
		defer roleRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository).Execute(userUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure on getting user's roles", func(t *testing.T) {
		var (
			roleRepository roles.MockRolesRepository

			userUUID = "test-user-uuid"

			expectedErr = errors.New("some error")
		)

		roleRepository.On("GetByUserUUID", userUUID).Once().Return(nil, expectedErr)
		defer roleRepository.AssertExpectations(t)

		response, err := NewUseCase(&roleRepository).Execute(userUUID)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
