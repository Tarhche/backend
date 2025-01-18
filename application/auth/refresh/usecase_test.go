package refresh

import (
	"errors"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("generates a fresh jwt token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{UUID: "test-uuid"}
			r = Request{
				Token: generateRefreshToken(t, j, u, time.Now().Add(15*time.Second), auth.RefreshToken),
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)

		accessTokenClaims, err := j.Verify(response.AccessToken)
		assert.NoError(t, err)
		assert.NotNil(t, accessTokenClaims)

		audience, err := accessTokenClaims.GetAudience()
		assert.NoError(t, err)
		assert.Equal(t, "permission", audience[0])

		refreshTokenClaims, err := j.Verify(response.RefreshToken)
		assert.NoError(t, err)
		assert.NotNil(t, accessTokenClaims)

		audience, err = refreshTokenClaims.GetAudience()
		assert.NoError(t, err)
		assert.Equal(t, "refresh", audience[0])
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"token": "token is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertExpectations(t)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 1)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("token is not valid", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{UUID: "test-uuid"}
			r = Request{
				Token: generateRefreshToken(t, j, u, time.Now().Add(-10*time.Second), auth.RefreshToken),
			}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"token": "token has invalid claims: token is expired",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 1)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on fetching user's data", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{UUID: "test-uuid"}
			r = Request{
				Token: generateRefreshToken(t, j, u, time.Now().Add(15*time.Second), auth.RefreshToken),
			}
			expectedErr = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}

func generateRefreshToken(t *testing.T, j *jwt.JWT, u user.User, expiresAt time.Time, audience string) string {
	t.Helper()

	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now().Add(-time.Hour))
	b.SetExpirationTime(expiresAt)
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{audience})

	token, err := j.Generate(b.Build())

	assert.NoError(t, err)

	return token
}
