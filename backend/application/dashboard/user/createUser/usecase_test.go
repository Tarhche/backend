package createuser

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeOf(func(*translatorContract.Params) {}).Name()

	t.Run("successfully create a user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
				Password: "test",
			}

			userUUID = "test-user-uuid"
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock2.Anything).Once().Return(userUUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock2.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"))
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
		assert.Equal(t, userUUID, response.UUID)
	})

	t.Run("invalid request", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"email":    "email is required",
					"name":     "name is required",
					"password": "password is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")

		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("another user with same email exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
				Password: "test",
			}

			u = user.User{
				Email: r.Email,
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"email": "another user with same email already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["email"],
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["email"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity", r.Username)
		userRepository.AssertNotCalled(t, "Save")

		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("another user with same username exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
				Password: "test",
			}

			u = user.User{
				Username: r.Username,
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"username": "another user with same username already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["username"],
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["username"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		userRepository.AssertNotCalled(t, "Save")

		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("failure on fetching userinfo", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
				Password: "test",
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		userRepository.AssertNotCalled(t, "GetOneByIdentity", r.Username)
		userRepository.AssertNotCalled(t, "Save")

		hasher.AssertNotCalled(t, "Hash")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on saving user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r = Request{
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
				Password: "test",
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock2.Anything).Once().Return("", expectedError)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock2.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"))
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
