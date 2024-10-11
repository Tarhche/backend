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

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/updateprofile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUpdateProfileHandler(t *testing.T) {
	t.Run("update profile", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = updateprofile.Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, nil)
		userRepository.On("Save", &u).Once().Return(r.UserUUID, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewUpdateProfileHandler(updateprofile.NewUseCase(&userRepository))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = updateprofile.Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		handler := NewUpdateProfileHandler(updateprofile.NewUseCase(&userRepository))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/update-profile-validation-errors-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = updateprofile.Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		userRepository.On("GetOne", r.UserUUID).Once().Return(u, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		handler := NewUpdateProfileHandler(updateprofile.NewUseCase(&userRepository))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			r = updateprofile.Request{
				UserUUID: "test-user-uuid",
				Name:     "John Doe",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "john.doe",
			}

			u = user.User{
				UUID:     r.UserUUID,
				Name:     r.Name,
				Avatar:   r.Avatar,
				Email:    r.Email,
				Username: r.Username,
			}
		)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(u, errors.New("unexpected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewUpdateProfileHandler(updateprofile.NewUseCase(&userRepository))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
