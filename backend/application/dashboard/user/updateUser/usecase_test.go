package updateuser

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("successfully update a user", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock2.Anything).Once().Return(r.UserUUID, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("invalid request", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"email":    "email is required",
					"name":     "name is required",
					"password": "password is required",
				},
			}
		)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("another user with same email exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			u = user.User{
				Email: r.Email,
			}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"email": "another user with same email already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity", r.Username)
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("another user with same username exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			u = user.User{
				Username: r.Username,
			}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"username": "another user with same username already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, &expectedResponse, response)
	})

	t.Run("failure on fetching userinfo by identity", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			expectedError = errors.New("some error")
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity", r.Username)
		userRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on fetching userinfo by uuid", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			expectedError = errors.New("some error")
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on saving user", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Email:    r.Email,
				Username: r.Username,
			}

			expectedError = errors.New("some error")
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock2.Anything).Once().Return("", expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
