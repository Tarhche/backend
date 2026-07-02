package getuser

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting a user succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"
			a        = user.User{
				UUID: userUUID,
			}
			expectedResponse = Response{
				UUID: userUUID,
			}
		)

		userRepository.On("GetOne", mock.Anything, userUUID).Return(a, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(context.Background(), userUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting a user fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID      = "user-uuid"
			expectedError = errors.New("error")
		)

		userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(context.Background(), userUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
