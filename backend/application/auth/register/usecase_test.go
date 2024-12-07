package register

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeOf(func(*translatorContract.Params) {}).Name()

	t.Run("sends registration mail", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			r = Request{
				Identity: "test@mail.com",
			}

			command = SendRegistrationEmail{
				Identity: r.Identity,
			}
		)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendRegisterationEmailName, payload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			r = Request{
				Identity: "somethingForTest",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"identity": "identity is not a valid email address",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			r = Request{
				Identity: "test@mail.com",
			}

			u = user.User{
				Email: r.Identity,
			}

			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"identity": "user with given email already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			"email_already_exists",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["identity"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&r)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get user fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			r = Request{
				Identity: "test@mail.com",
			}

			u = user.User{
				Email: r.Identity,
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("publishing registeration mail command fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			r = Request{
				Identity: "test@mail.com",
			}

			command = SendRegistrationEmail{
				Identity: r.Identity,
			}

			expectedError = errors.New("some error")
		)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendRegisterationEmailName, payload).Return(expectedError)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
