package login

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("login succeeds", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto

			request = Request{
				Identity: "test-identity",
				Password: "test-password",
			}

			u = user.User{
				UUID: request.Identity,
				PasswordHash: password.Hash{
					Value: []byte("hashed-value"),
					Salt:  []byte("salt-value"),
				},
			}
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(request.Password), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher).Execute(request)

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
		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto

			request = Request{}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"identity": "identity required",
					"password": "password required",
				},
			}
		)

		response, err := NewUseCase(&userRepository, j, &hasher).Execute(request)

		assert.NoError(t, err)
		assert.NotNil(t, response)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		hasher.AssertNotCalled(t, "Equal")

		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("finding user fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto

			request = Request{
				Identity: "test-identity",
				Password: "test-password",
			}

			expectedError = errors.New("test error")
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher).Execute(request)

		hasher.AssertNotCalled(t, "Equal")

		assert.ErrorIs(t, expectedError, err)
		assert.Nil(t, response)
	})

	t.Run("invalid password", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto

			request = Request{
				Identity: "test-identity",
				Password: "test-password",
			}

			u = user.User{
				UUID: request.Identity,
				PasswordHash: password.Hash{
					Value: []byte("hashed-value"),
					Salt:  []byte("salt-value"),
				},
			}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"identity": "your identity or password is wrong",
				},
			}
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(request.Password), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(false)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher).Execute(request)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})
}
