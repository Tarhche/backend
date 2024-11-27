package getprofile

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("gets user info", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			u = user.User{
				UUID:     userUUID,
				Name:     "test name",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "test-username",
			}

			expectedResponse = Response{
				UUID:     u.UUID,
				Name:     u.Name,
				Avatar:   u.Avatar,
				Email:    u.Email,
				Username: u.Username,
			}
		)

		userRepository.On("GetOne", userUUID).Once().Return(u, nil)

		response, err := NewUseCase(&userRepository).Execute(userUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting user info fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			expectedErr = errors.New("user not found")
		)

		userRepository.On("GetOne", userUUID).Once().Return(user.User{}, expectedErr)

		response, err := NewUseCase(&userRepository).Execute(userUUID)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
