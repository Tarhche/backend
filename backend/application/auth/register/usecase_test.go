package register

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
	t.Run("sends registration mail", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = Request{
				Identity: "test@mail.com",
			}

			command = SendRegistrationEmail{
				Identity: r.Identity,
			}
		)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendRegisterationEmailName, payload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = Request{
				Identity: "somethingForTest",
			}
		)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 1)
	})

	t.Run("user exists", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

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

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(r)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get user fails", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = Request{
				Identity: "test@mail.com",
			}

			u = user.User{
				Email: r.Identity,
			}

			expectedError = errors.New("some error")
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(r)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("publishing registeration mail command fails", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

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

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), SendRegisterationEmailName, payload).Return(expectedError)
		defer asyncCommandBus.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &asyncCommandBus).Execute(r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
