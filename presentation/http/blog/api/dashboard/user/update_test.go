package user

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateHandler(t *testing.T) {
	t.Parallel()

	t.Run("create user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			requestValidator validator.MockValidator

			u = user.User{
				UUID: "user-uuid",
			}

			r = updateuser.Request{
				UserUUID: "test-user-uuid",
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
			}

			updatedUser = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOne", r.UserUUID).Once().Return(updatedUser, nil)
		userRepository.On("Save", mock2.Anything).Once().Return(r.UserUUID, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updateuser.NewUseCase(&userRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
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
			requestValidator validator.MockValidator

			u = user.User{
				UUID: "user-uuid",
			}
		)

		requestValidator.On("Validate", &updateuser.Request{}).Once().Return(domain.ValidationErrors{
			"email": "email is required",
			"name":  "name is required",
			"uuid":  "universal unique identifier (uuid) is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewUpdateHandler(updateuser.NewUseCase(&userRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		expectedBody, err := os.ReadFile("testdata/update-users-validation-failed-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
