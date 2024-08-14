package changepassword

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("change password successfully", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
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
					"uuid":             "user's universal unique identifier (uuid) is required",
					"current_password": "current password is required",
					"new_password":     "password is required",
				},
			}
		)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
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
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			expectedErr = errors.New("error")
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
		hasher.AssertNotCalled(t, "Hash")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("invalid (current) password", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"current_password": "current password is not valid",
				},
			}
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(false)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		hasher.AssertNotCalled(t, "Hash")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("saving user failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto

			r = Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}

			expectedError = errors.New("error")
		)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return("", expectedError)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher).Execute(r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
