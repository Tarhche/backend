package user

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
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestChangePasswordHandler(t *testing.T) {
	t.Run("change password", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			authorizer     domain.MockAuthorizer

			r = userchangepassword.Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			u = user.User{
				UUID: r.UserUUID,
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersPasswordUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		handler := NewChangePasswordHandler(userchangepassword.NewUseCase(&userRepository, &hasher), &authorizer)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersPasswordUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewChangePasswordHandler(userchangepassword.NewUseCase(&userRepository, &hasher), &authorizer)

		request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		expectedBody, err := os.ReadFile("testdata/user-change-password-validation-failed-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("unauthorised", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersPasswordUpdate).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewChangePasswordHandler(userchangepassword.NewUseCase(&userRepository, &hasher), &authorizer)

		request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypt.MockCrypto
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersPasswordUpdate).Once().Return(false, errors.New("undexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewChangePasswordHandler(userchangepassword.NewUseCase(&userRepository, &hasher), &authorizer)

		request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
