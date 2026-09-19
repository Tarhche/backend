package getprofile

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("gets user info", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			u = user.User{
				UUID:         userUUID,
				Name:         "test name",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "EN",
			}

			expectedResponse = Response{
				UUID:         u.UUID,
				Name:         u.Name,
				Avatar:       u.Avatar,
				Email:        u.Email,
				Username:     u.Username,
				LanguageCode: u.LanguageCode,
			}
		)

		userRepository.On("GetOne", mock.Anything, userUUID).Once().Return(u, nil)

		response, err := NewUseCase(&userRepository).Execute(context.Background(), &Request{UserUUID: userUUID})

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting user info fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			expectedErr = errors.New("user not found")
		)

		userRepository.On("GetOne", mock.Anything, userUUID).Once().Return(user.User{}, expectedErr)

		response, err := NewUseCase(&userRepository).Execute(context.Background(), &Request{UserUUID: userUUID})

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("a profile says who is seeing it as this user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID:         "user-uuid",
				Name:         "test name",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "EN",
			}

			impersonator = user.User{
				UUID:     "impersonator-uuid",
				Name:     "impersonator name",
				Avatar:   "impersonator-avatar",
				Email:    "impersonator@test.com",
				Username: "impersonator-username",
			}

			expectedResponse = Response{
				UUID:         u.UUID,
				Name:         u.Name,
				Avatar:       u.Avatar,
				Email:        u.Email,
				Username:     u.Username,
				LanguageCode: u.LanguageCode,
				ImpersonatedBy: &impersonatorResponse{
					UUID:     impersonator.UUID,
					Name:     impersonator.Name,
					Avatar:   impersonator.Avatar,
					Username: impersonator.Username,
				},
			}
		)

		userRepository.On("GetOne", mock.Anything, u.UUID).Once().Return(u, nil)
		userRepository.On("GetOne", mock.Anything, impersonator.UUID).Once().Return(impersonator, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(context.Background(), &Request{
			UserUUID:         u.UUID,
			ImpersonatorUUID: impersonator.UUID,
		})

		assert.NoError(t, err)
		// the profile is still the impersonated user's; only the extra field is
		// about anybody else
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("looking up who is behind the session fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			u           = user.User{UUID: "user-uuid"}
			expectedErr = errors.New("impersonator not found")
		)

		userRepository.On("GetOne", mock.Anything, u.UUID).Once().Return(u, nil)
		userRepository.On("GetOne", mock.Anything, "impersonator-uuid").Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(context.Background(), &Request{
			UserUUID:         u.UUID,
			ImpersonatorUUID: "impersonator-uuid",
		})

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
