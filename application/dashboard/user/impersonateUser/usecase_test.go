package impersonateuser

import (
	"context"
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	rl := []role.Role{
		{
			UUID:        "role-uuid-1",
			Name:        "role-1",
			Permissions: []string{"permission-1", "permission-2"},
		},
	}

	t.Run("a session that acts as somebody else", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{
				UUID:         "user-uuid",
				Name:         "test name",
				Avatar:       "test-avatar",
				Email:        "test@test.com",
				Username:     "test-username",
				LanguageCode: "FA",
			}

			request = Request{
				UserUUID:         u.UUID,
				ImpersonatorUUID: "impersonator-uuid",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		// the permissions the tokens carry are the impersonated user's, not the
		// impersonator's: that is the whole point of the session
		roleRepository.On("GetByUserUUID", mock.Anything, u.UUID).Once().Return(rl, nil)
		defer roleRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, auth.NewTokenGenerator(j, &roleRepository), &translator, &validator).
			Execute(context.Background(), &request)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
		assert.Equal(t, &userResponse{
			UUID:     u.UUID,
			Name:     u.Name,
			Avatar:   u.Avatar,
			Email:    u.Email,
			Username: u.Username,
		}, response.User)

		accessTokenClaims, err := j.Verify(context.Background(), response.AccessToken)
		assert.NoError(t, err)

		subject, err := accessTokenClaims.GetSubject()
		assert.NoError(t, err)
		assert.Equal(t, u.UUID, subject)

		audience, err := accessTokenClaims.GetAudience()
		assert.NoError(t, err)
		assert.Equal(t, auth.AccessToken, audience[0])

		claimsMap, ok := accessTokenClaims.(jwtv5.MapClaims)
		assert.True(t, ok)
		assert.ElementsMatch(t, rl[0].Permissions, claimsMap["permissions"])
		assert.Equal(t, u.LanguageCode, claimsMap["lang"], "the dashboard is shown in the language of whoever is being seen")
		assert.Equal(t, request.ImpersonatorUUID, jwt.Impersonator(accessTokenClaims))

		// the refresh token says it too, so the session stays a shadow one
		refreshTokenClaims, err := j.Verify(context.Background(), response.RefreshToken)
		assert.NoError(t, err)
		assert.Equal(t, request.ImpersonatorUUID, jwt.Impersonator(refreshTokenClaims))

		audience, err = refreshTokenClaims.GetAudience()
		assert.NoError(t, err)
		assert.Equal(t, auth.RefreshToken, audience[0])
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{
				ImpersonatorUUID: "impersonator-uuid",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"uuid": "this field is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, auth.NewTokenGenerator(j, &roleRepository), &translator, &validator).
			Execute(context.Background(), &request)

		userRepository.AssertNotCalled(t, "GetOne")
		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("what is refused before anybody is looked up", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			request Request
			message string
		}{
			{
				name: "being seen as somebody does not carry the right to be seen as a third person",
				request: Request{
					UserUUID:                     "another-user-uuid",
					ImpersonatorUUID:             "user-uuid",
					CallerIsAlreadyImpersonating: true,
				},
				message: "impersonation_does_not_nest",
			},
			{
				name: "one is already oneself",
				request: Request{
					UserUUID:         "user-uuid",
					ImpersonatorUUID: "user-uuid",
				},
				message: "already_signed_in_as_this_user",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				var (
					userRepository users.MockUsersRepository
					roleRepository roles.MockRolesRepository
					validator      validator.MockValidator
					translator     translator.TranslatorMock

					expectedResponse = Response{
						ValidationErrors: domain.ValidationErrors{
							"uuid": "refused",
						},
					}
				)

				validator.On("Validate", &test.request).Once().Return(nil)
				defer validator.AssertExpectations(t)

				translator.On("Translate", test.message, mock.Anything).Once().Return("refused")
				defer translator.AssertExpectations(t)

				response, err := NewUseCase(&userRepository, auth.NewTokenGenerator(j, &roleRepository), &translator, &validator).
					Execute(context.Background(), &test.request)

				userRepository.AssertNotCalled(t, "GetOne")
				roleRepository.AssertNotCalled(t, "GetByUserUUID")

				assert.NoError(t, err)
				assert.Equal(t, &expectedResponse, response)
				assert.Empty(t, response.AccessToken)
				assert.Empty(t, response.RefreshToken)
			})
		}
	})

	t.Run("nobody to be seen as", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{
				UserUUID:         "user-uuid",
				ImpersonatorUUID: "impersonator-uuid",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"uuid": "identity (email/username) not exists",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, request.UserUUID).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		translator.On("Translate", "identity_not_exists", mock.Anything).Once().
			Return(expectedResponse.ValidationErrors["uuid"])
		defer translator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, auth.NewTokenGenerator(j, &roleRepository), &translator, &validator).
			Execute(context.Background(), &request)

		roleRepository.AssertNotCalled(t, "GetByUserUUID")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("a banned user's shoes are nobody's to stand in", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				BannedAt: time.Now().Add(-time.Hour),
			}

			request = Request{
				UserUUID:         u.UUID,
				ImpersonatorUUID: "impersonator-uuid",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"uuid": "the account has been blocked",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		translator.On("Translate", "user_is_banned", mock.Anything).Once().
			Return(expectedResponse.ValidationErrors["uuid"])
		defer translator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, auth.NewTokenGenerator(j, &roleRepository), &translator, &validator).
			Execute(context.Background(), &request)

		roleRepository.AssertNotCalled(t, "GetByUserUUID")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
		assert.Empty(t, response.AccessToken)
	})

	t.Run("finding the user fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{
				UserUUID:         "user-uuid",
				ImpersonatorUUID: "impersonator-uuid",
			}

			expectedErr = errors.New("some error")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, request.UserUUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, auth.NewTokenGenerator(j, &roleRepository), &translator, &validator).
			Execute(context.Background(), &request)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
