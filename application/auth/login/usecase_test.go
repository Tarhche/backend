package login

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	var translatorOptionsType = reflect.TypeOf(func(*translatorContract.Params) {}).Name()

	t.Run("login succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

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

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(request.Password), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher, &translator, &validator).Execute(&request)

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
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"identity": "identity required",
					"password": "password required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher, &translator, &validator).Execute(&request)

		assert.NoError(t, err)
		assert.NotNil(t, response)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		hasher.AssertNotCalled(t, "Equal")

		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("finding user fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{
				Identity: "test-identity",
				Password: "test-password",
			}

			expectedError = errors.New("test error")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")
		hasher.AssertNotCalled(t, "Equal")

		assert.ErrorIs(t, expectedError, err)
		assert.Nil(t, response)
	})

	t.Run("invalid password", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         mock.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

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
				ValidationErrors: domain.ValidationErrors{
					"identity": "your identity or password is wrong",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			"invalid_identity_or_password",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["identity"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(request.Password), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(false)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &hasher, &translator, &validator).Execute(&request)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})
}
