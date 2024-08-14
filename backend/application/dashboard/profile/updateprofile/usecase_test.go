package updateprofile

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("profile is updated successfully", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", &u).Once().Return(r.UserUUID, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"uuid":     "universal unique identifier (uuid) is required",
					"name":     "name is required",
					"email":    "email is required",
					"username": "username is required",
				},
			}
		)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure because another user with given email exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     "test-user-uuid-1",
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}

			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"email": "another user with this email already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity", r.Username)
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure because another user with given username exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
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
				ValidationErrors: map[string]string{
					"username": "another user with this email already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(anotherUserWithSameUsername, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting user info fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
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

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		userRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving user info fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}

			expectedErr = errors.New("save user info error")
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", &u).Once().Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
