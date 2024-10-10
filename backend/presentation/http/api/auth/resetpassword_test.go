package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/auth/resetpassword"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	crypto "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestResetPasswordHandler(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("reset password", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			u = user.User{
				UUID: "test-uuid",
			}

			r = resetpassword.Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(1*time.Minute), auth.ResetPasswordToken),
				Password: "test-password",
			}
		)

		userRepository.On("GetOne", u.UUID).Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock.AnythingOfType("[]uint8")).Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		handler := NewResetPasswordHandler(resetpassword.NewUseCase(&userRepository, &hasher, j))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
		)

		handler := NewResetPasswordHandler(resetpassword.NewUseCase(&userRepository, &hasher, j))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		expected, err := os.ReadFile("testdata/resetpassword-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("user not exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			u = user.User{
				UUID: "test-uuid",
			}

			r = resetpassword.Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(1*time.Minute), auth.ResetPasswordToken),
				Password: "test-password",
			}
		)

		userRepository.On("GetOne", u.UUID).Return(user.User{}, domain.ErrNotExists)

		handler := NewResetPasswordHandler(resetpassword.NewUseCase(&userRepository, &hasher, j))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		expected, err := os.ReadFile("testdata/resetpassword-response-user-not-exists.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			u = user.User{
				UUID: "test-uuid",
			}

			r = resetpassword.Request{
				Token:    resetPasswordToken(t, j, u, time.Now().Add(1*time.Minute), auth.ResetPasswordToken),
				Password: "test-password",
			}
		)

		userRepository.On("GetOne", u.UUID).Return(user.User{}, errors.New("unexpected error"))

		handler := NewResetPasswordHandler(resetpassword.NewUseCase(&userRepository, &hasher, j))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
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
