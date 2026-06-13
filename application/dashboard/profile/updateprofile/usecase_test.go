package updateprofile

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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

	t.Run("profile is updated successfully", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "John Doe",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "john.doe",
				LanguageCode: "EN",
			}

			u = user.User{
				UUID:         r.UserUUID,
				Name:         r.Name,
				Avatar:       r.Avatar,
				Email:        r.Email,
				Username:     r.Username,
				LanguageCode: r.LanguageCode,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("Verify", r.LanguageCode).Once().Return(true)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", &u).Once().Return(r.UserUUID, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"uuid":          "universal unique identifier (uuid) is required",
					"name":          "name is required",
					"email":         "email is required",
					"username":      "username is required",
					"language_code": "language is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		languageResolver.AssertNotCalled(t, "Verify")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure because another user with given email exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "John Doe",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "john.doe",
				LanguageCode: "EN",
			}

			u = user.User{
				UUID:     "test-user-uuid-1",
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"email": "another user with this email already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			"email_already_exists",
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["email"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		languageResolver.AssertNotCalled(t, "Verify")
		userRepository.AssertNotCalled(t, "GetOneByIdentity", r.Username)
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure because another user with given username exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "John Doe",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "john.doe",
				LanguageCode: "EN",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}

			anotherUserWithSameUsername = user.User{
				UUID:     "test-user-uuid-1",
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"username": "another user with this email already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			"username_already_exists",
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["username"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(anotherUserWithSameUsername, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		languageResolver.AssertNotCalled(t, "Verify")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure because the chosen language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "John Doe",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "john.doe",
				LanguageCode: "DE",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
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
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["language_code"])
		defer translator.AssertExpectations(t)

		languageResolver.On("Verify", r.LanguageCode).Once().Return(false)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting user info fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "John Doe",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "john.doe",
				LanguageCode: "EN",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}

			expectedErr = errors.New("get user info error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("Verify", r.LanguageCode).Once().Return(true)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving user info fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			languageResolver resolver.MockResolver
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{
				UserUUID:     "test-user-uuid",
				Name:         "John Doe",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "john.doe",
				LanguageCode: "EN",
			}

			u = user.User{
				UUID:         r.UserUUID,
				Name:         r.Name,
				Avatar:       r.Avatar,
				Email:        r.Email,
				Username:     r.Username,
				LanguageCode: r.LanguageCode,
			}

			expectedErr = errors.New("save user info error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("Verify", r.LanguageCode).Once().Return(true)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", &u).Once().Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &languageResolver, &validator, &translator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
