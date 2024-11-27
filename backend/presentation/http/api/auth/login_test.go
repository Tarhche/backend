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
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth/login"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	crypto "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestLoginHandler(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("successfully login", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

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

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher, &translator, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.Contains(t, response.Body.String(), "access_token")
		assert.Contains(t, response.Body.String(), "refresh_token")
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock
		)

		requestValidator.On("Validate", &login.Request{}).Once().Return(domain.ValidationErrors{
			"identity": "identity required",
			"password": "password required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher, &translator, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		hasher.AssertNotCalled(t, "Equal")

		expected, err := os.ReadFile("testdata/login-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = login.Request{
				Identity: "some test identity",
				Password: "some random password",
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translator.On("Translate", "identity (email/username) or password is wrong", mock.Anything).Once().Return("identity (email/username) or password is wrong")
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher, &translator, &requestValidator))

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
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = login.Request{
				Identity: "some test identity",
				Password: "some random password",
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("an unexpected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewLoginHandler(login.NewUseCase(&userRepository, j, &hasher, &translator, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		hasher.AssertNotCalled(t, "Equal")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
