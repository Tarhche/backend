package changepassword

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeOf(func(*translatorContract.Params) {}).Name()

	t.Run("change password successfully", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

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

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"uuid":             "user's universal unique identifier (uuid) is required",
					"current_password": "current password is required",
					"new_password":     "password is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
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
			translator     translator.TranslatorMock

			r = Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
		hasher.AssertNotCalled(t, "Hash")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("invalid (current) password", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

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
				ValidationErrors: domain.ValidationErrors{
					"current_password": "current password is not valid",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["current_password"],
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["current_password"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(false)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		hasher.AssertNotCalled(t, "Hash")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("saving user failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

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

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return("", expectedError)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
