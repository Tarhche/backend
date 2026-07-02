package updateuser

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeFor[func(*translatorContract.Params)]().Name()

	t.Run("successfully update a user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "en",
			}

			u = user.User{
				UUID:         r.UserUUID,
				Name:         r.Name,
				Email:        r.Email,
				Username:     r.Username,
				LanguageCode: r.LanguageCode,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("Verify", mock2.Anything, r.LanguageCode).Once().Return(true)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock2.Anything, r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", mock2.Anything, r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock2.Anything, mock2.Anything).Once().Return(r.UserUUID, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("invalid request", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

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

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		translator.AssertNotCalled(t, "Translate")

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		languageResolver.AssertNotCalled(t, "Verify")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("another user with same email exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "en",
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
			"email_already_exists",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["email"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity", mock2.Anything, r.Username)
		userRepository.AssertNotCalled(t, "Save")
		languageResolver.AssertNotCalled(t, "Verify")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("another user with same username exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "en",
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
			"username_already_exists",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["username"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock2.Anything, r.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		userRepository.AssertNotCalled(t, "Save")
		languageResolver.AssertNotCalled(t, "Verify")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("invalid language code", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "invalid-language-code",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"language_code": "language code is invalid",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			"invalid_value",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["language_code"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock2.Anything, r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		languageResolver.On("Verify", mock2.Anything, r.LanguageCode).Once().Return(false)
		defer languageResolver.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure on fetching userinfo by identity", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "en",
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		translator.AssertNotCalled(t, "Translate")

		userRepository.AssertNotCalled(t, "GetOneByIdentity", mock2.Anything, r.Username)
		userRepository.AssertNotCalled(t, "Save")
		languageResolver.AssertNotCalled(t, "Verify")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on fetching userinfo by uuid", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "en",
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("Verify", mock2.Anything, r.LanguageCode).Once().Return(true)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock2.Anything, r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", mock2.Anything, r.UserUUID).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		userRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on saving user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "test name",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "en",
			}

			u = user.User{
				UUID:         r.UserUUID,
				Name:         r.Name,
				Email:        r.Email,
				Username:     r.Username,
				LanguageCode: r.LanguageCode,
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("Verify", mock2.Anything, r.LanguageCode).Once().Return(true)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock2.Anything, r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock2.Anything, r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", mock2.Anything, r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock2.Anything, mock2.Anything).Once().Return("", expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
