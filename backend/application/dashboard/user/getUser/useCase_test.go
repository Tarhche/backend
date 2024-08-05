package getuser

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("getting a user succeeds", func(t *testing.T) {
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

		userRepository.On("GetOne", userUUID).Return(a, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(userUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting a user fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			userUUID      = "user-uuid"
			expectedError = errors.New("error")
		)

		userRepository.On("GetOne", userUUID).Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(userUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
