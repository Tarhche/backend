package userchangepassword

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("update password", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
			}
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"uuid":         "universal unique identifier (uuid) is required",
					"new_password": "password is required",
				},
			}
		)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting userinfo fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			expectedErr = errors.New("something went wrong")
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving userinfo fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
			}

			expectedErr = errors.New("something went wrong")
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
