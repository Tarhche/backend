package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth/login"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	crypto "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestLoginHandler(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("successfully login", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			r = login.Request{
				Identity: "some test identity",
				Password: "some random password",
			}

			u = user.User{
				UUID:     "user-uuid",
				Email:    "user-email",
				Username: r.Identity,
				PasswordHash: password.Hash{
					Value: []byte("value"),
					Salt:  []byte("salt"),
				},
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.Password), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		defer hasher.AssertExpectations(t)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.Contains(t, response.Body.String(), "access_token")
		assert.Contains(t, response.Body.String(), "refresh_token")
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto
		)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		hasher.AssertNotCalled(t, "Equal")

		expected, err := os.ReadFile("testdata/login-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("user not found", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			r = login.Request{
				Identity: "some test identity",
				Password: "some random password",
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		hasher.AssertNotCalled(t, "Equal")

		expected, err := os.ReadFile("testdata/login-response-user-not-found.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			r = login.Request{
				Identity: "some test identity",
				Password: "some random password",
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("an unexpected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		hasher.AssertNotCalled(t, "Equal")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
