package deleteuser

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deleting a user succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			r = Request{UserUUID: "user-uuid"}
		)

		userRepository.On("Delete", mock.Anything, r.UserUUID).Return(nil)
		defer userRepository.AssertExpectations(t)

		err := NewUseCase(&userRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting a user fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			r             = Request{UserUUID: "user-uuid"}
			expectedError = errors.New("user deletion failed")
		)

		userRepository.On("Delete", mock.Anything, r.UserUUID).Return(expectedError)
		defer userRepository.AssertExpectations(t)

		err := NewUseCase(&userRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
