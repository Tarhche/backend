package auth

import (
	"bytes"
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
	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestRefreshHandler(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	var generateRefreshToken = func(u user.User) (string, error) {
		b := jwt.NewClaimsBuilder()
		b.SetSubject(u.UUID)
		b.SetNotBefore(time.Now())
		b.SetExpirationTime(time.Now().Add(2 * 24 * time.Hour))
		b.SetIssuedAt(time.Now())
		b.SetAudience([]string{auth.RefreshToken})

		return j.Generate(b.Build())
	}

	t.Run("refresh token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				Email:    "user-email",
				Username: "user-username",
				PasswordHash: password.Hash{
					Value: []byte("value"),
					Salt:  []byte("salt"),
				},
			}
		)

		userRepository.On("GetOne", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		refreshToken, err := generateRefreshToken(u)
		assert.NoError(t, err)

		r := refresh.Request{
			Token: refreshToken,
		}

		var payload bytes.Buffer
		err = json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		handler := NewRefreshHandler(refresh.NewUseCase(&userRepository, j, &translator, &requestValidator))

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
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock
		)

		requestValidator.On("Validate", &refresh.Request{}).Once().Return(domain.ValidationErrors{
			"token": "token is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewRefreshHandler(refresh.NewUseCase(&userRepository, j, &translator, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")

		expected, err := os.ReadFile("testdata/refresh-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				Email:    "user-email",
				Username: "user-username",
				PasswordHash: password.Hash{
					Value: []byte("value"),
					Salt:  []byte("salt"),
				},
			}
		)

		userRepository.On("GetOne", u.UUID).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		refreshToken, err := generateRefreshToken(u)
		assert.NoError(t, err)

		r := refresh.Request{
			Token: refreshToken,
		}

		var payload bytes.Buffer
		err = json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translator.On("Translate", "identity (email/username) not exists", mock.Anything).Once().Return("identity (email/username) not exists")
		defer translator.AssertExpectations(t)

		handler := NewRefreshHandler(refresh.NewUseCase(&userRepository, j, &translator, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/refresh-response-user-not-found.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				Email:    "user-email",
				Username: "user-username",
				PasswordHash: password.Hash{
					Value: []byte("value"),
					Salt:  []byte("salt"),
				},
			}
		)

		refreshToken, err := generateRefreshToken(u)
		assert.NoError(t, err)

		r := refresh.Request{
			Token: refreshToken,
		}

		var payload bytes.Buffer
		err = json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Once().Return(user.User{}, errors.New("something unexpected"))
		defer userRepository.AssertExpectations(t)

		handler := NewRefreshHandler(refresh.NewUseCase(&userRepository, j, &translator, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
