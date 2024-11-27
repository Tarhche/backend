package forgetpassword

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

	t.Run("successfully mails reset-password token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			u = user.User{
				UUID:  "user-uuid",
				Email: request.Identity,
			}
		)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendForgetPasswordEmailName, payload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&request)

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

			request          = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"identity": "identity is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"identity": "identity (email/username) not exists",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["identity"],
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["identity"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&request)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on finding user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			expectedErr = errors.New("something bad happened")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

	t.Run("error on publishing sendmail command", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
			validator       validator.MockValidator
			translator      translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			u = user.User{
				UUID:  "user-uuid",
				Email: request.Identity,
			}

			expectedErr = errors.New("something bad happened")
		)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendForgetPasswordEmailName, payload).Return(expectedErr)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

}
