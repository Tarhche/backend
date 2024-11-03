package forgetpassword

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("successfully mails reset-password token", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

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

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendForgetPasswordEmailName, payload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(request)
		assert.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			request          = Request{}
			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"identity": "identity is required",
				},
			}
		)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user not found", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			request = Request{
				Identity: "something@somewhere.loc",
			}
			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"identity": "identity (email/username) not exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(request)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on finding user", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			request = Request{
				Identity: "something@somewhere.loc",
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(request)

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

	t.Run("error on publishing sendmail command", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

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

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendForgetPasswordEmailName, payload).Return(expectedErr)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(request)
		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

}
