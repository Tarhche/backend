package resetpassword

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	crypto "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_ResetPassword(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("updates password successfully", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{
				UUID: "test-uuid",
			}

			request = Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(1*time.Minute), auth.ResetPasswordToken),
				Password: "test-password",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(request.Password), mock.AnythingOfType("[]uint8")).Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"token":    "token is required",
					"password": "password is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("invalid base64 token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			request = Request{
				Token:    "invalid-base64-token",
				Password: "test-password",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.Error(t, err)
		assert.Nil(t, response)
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{UUID: "test-uuid"}

			request = Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(-2*time.Second), auth.ResetPasswordToken),
				Password: "test-password",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.Error(t, err)
		assert.Nil(t, response)
	})

	t.Run("error on fetching user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{UUID: "test-uuid"}

			request = Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(10*time.Second), auth.ResetPasswordToken),
				Password: "test-password",
			}

			expectedErr = errors.New("user not found")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")
		hasher.AssertNotCalled(t, "Hash")
		userRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("error on persisting user's password", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
			validator      validator.MockValidator
			translator     translator.TranslatorMock

			u = user.User{UUID: "test-uuid"}

			request = Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(10*time.Second), auth.ResetPasswordToken),
				Password: "test-password",
			}

			expectedErr = errors.New("user not found")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Return(u, nil)
		userRepository.On("Save", mock.Anything).Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(request.Password), mock.AnythingOfType("[]uint8")).Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j, &translator, &validator).Execute(&request)

		translator.AssertNotCalled(t, "Translate")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}

func resetPasswordToken(t *testing.T, j *jwt.JWT, u user.User, expiresAt time.Time, audience string) string {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now().Add(-time.Hour))
	b.SetExpirationTime(expiresAt)
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{audience})

	token, err := j.Generate(b.Build())
	assert.NoError(t, err)

	return base64.URLEncoding.EncodeToString([]byte(token))
}
