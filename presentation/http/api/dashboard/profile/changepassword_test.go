package profile

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

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/changepassword"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestChangePasswordHandler(t *testing.T) {
	t.Parallel()

	t.Run("change password", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypt.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = changepassword.Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Equal", []byte(r.CurrentPassword), u.PasswordHash.Value, u.PasswordHash.Salt).Once().Return(true)
		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		handler := NewChangePasswordHandler(changepassword.NewUseCase(&userRepository, &hasher, &requestValidator, &translator))

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypt.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}
		)

		requestValidator.On("Validate", &changepassword.Request{UserUUID: u.UUID}).Once().Return(domain.ValidationErrors{
			"current_password": "current password is required",
			"new_password":     "password is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewChangePasswordHandler(changepassword.NewUseCase(&userRepository, &hasher, &requestValidator, &translator))

		request := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
		hasher.AssertNotCalled(t, "Hash")

		expected, err := os.ReadFile("testdata/change-password-validation-errors-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypt.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = changepassword.Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		handler := NewChangePasswordHandler(changepassword.NewUseCase(&userRepository, &hasher, &requestValidator, &translator))

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
		hasher.AssertNotCalled(t, "Hash")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypt.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = changepassword.Request{
				UserUUID:        "user-uuid",
				CurrentPassword: "current-password",
				NewPassword:     "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
				PasswordHash: password.Hash{
					Value: []byte("hashed-current-password"),
					Salt:  []byte("current-password-salt"),
				},
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(user.User{}, errors.New("unexpected error"))
		defer userRepository.AssertExpectations(t)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		handler := NewChangePasswordHandler(changepassword.NewUseCase(&userRepository, &hasher, &requestValidator, &translator))

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Equal")
		hasher.AssertNotCalled(t, "Hash")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
