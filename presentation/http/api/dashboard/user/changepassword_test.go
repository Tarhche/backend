package user

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	crypt "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
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

			r = userchangepassword.Request{
				UserUUID:    "user-uuid",
				NewPassword: "new-password",
			}

			u = user.User{
				UUID: "logged-in-user-uuid",
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", mock.Anything).Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.NewPassword), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-new-password"), nil)
		defer hasher.AssertExpectations(t)

		handler := NewChangePasswordHandler(userchangepassword.NewUseCase(&userRepository, &hasher, &requestValidator))

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
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           crypt.MockCrypto
			requestValidator validator.MockValidator

			u = user.User{
				UUID: "logged-in-user-uuid",
			}

			validationErrors = domain.ValidationErrors{
				"uuid":         "this field is required",
				"new_password": "this field is required",
			}
		)

		requestValidator.On("Validate", &userchangepassword.Request{}).Once().Return(validationErrors)
		defer requestValidator.AssertExpectations(t)

		handler := NewChangePasswordHandler(userchangepassword.NewUseCase(&userRepository, &hasher, &requestValidator))

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
}
