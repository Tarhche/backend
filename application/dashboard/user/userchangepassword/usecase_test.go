package userchangepassword

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("update password", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator

			r = Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"uuid":         "this field is required",
					"new_password": "this field is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator).Execute(&r)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting userinfo fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator

			r = Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			expectedErr = errors.New("something went wrong")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator).Execute(&r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving userinfo fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator

			r = Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
			}

			expectedErr = errors.New("something went wrong")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
