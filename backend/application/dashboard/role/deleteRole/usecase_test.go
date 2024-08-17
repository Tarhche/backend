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
			elementRepository roles.MockRolesRepository

			r = Request{RoleUUID: "role-uuid"}
		)

		elementRepository.On("Delete", r.RoleUUID).Return(nil)
		defer elementRepository.AssertExpectations(t)

		err := NewUseCase(&elementRepository).Execute(r)

		assert.NoError(t, err)
	})

	t.Run("deleting the role fails", func(t *testing.T) {
		var (
			elementRepository roles.MockRolesRepository

			r             = Request{RoleUUID: "role-uuid"}
			expectedError = errors.New("role deletion failed")
		)

		elementRepository.On("Delete", r.RoleUUID).Return(expectedError)
		defer elementRepository.AssertExpectations(t)

		err := NewUseCase(&elementRepository).Execute(r)

		assert.ErrorIs(t, err, expectedError)
	})
}
