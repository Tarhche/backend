package deleterole

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes a role", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			r = Request{RoleUUID: "role-uuid"}
		)

		roleRepository.On("Delete", mock.Anything, r.RoleUUID).Return(nil)
		defer roleRepository.AssertExpectations(t)

		err := NewUseCase(&roleRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting the role fails", func(t *testing.T) {
		t.Parallel()

		var (
			roleRepository roles.MockRolesRepository

			r             = Request{RoleUUID: "role-uuid"}
			expectedError = errors.New("role deletion failed")
		)

		roleRepository.On("Delete", mock.Anything, r.RoleUUID).Return(expectedError)
		defer roleRepository.AssertExpectations(t)

		err := NewUseCase(&roleRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
