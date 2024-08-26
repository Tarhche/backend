package deleterole

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("deletes a role", func(t *testing.T) {
		var (
			roleRepository roles.MockRolesRepository

			r = Request{RoleUUID: "role-uuid"}
		)

		roleRepository.On("Delete", r.RoleUUID).Return(nil)
		defer roleRepository.AssertExpectations(t)

		err := NewUseCase(&roleRepository).Execute(r)

		assert.NoError(t, err)
	})

	t.Run("deleting the role fails", func(t *testing.T) {
		var (
			roleRepository roles.MockRolesRepository

			r             = Request{RoleUUID: "role-uuid"}
			expectedError = errors.New("role deletion failed")
		)

		roleRepository.On("Delete", r.RoleUUID).Return(expectedError)
		defer roleRepository.AssertExpectations(t)

		err := NewUseCase(&roleRepository).Execute(r)

		assert.ErrorIs(t, err, expectedError)
	})
}
