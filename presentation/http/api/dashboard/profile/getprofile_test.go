package profile

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/getprofile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestGetProfileHandler(t *testing.T) {
	t.Parallel()

	t.Run("get profile", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			u = user.User{
				UUID:     userUUID,
				Name:     "test name",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "test-username",
			}
		)

		userRepository.On("GetOne", userUUID).Once().Return(u, nil)

		handler := NewGetProfileHandler(getprofile.NewUseCase(&userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/get-profile-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			u = user.User{
				UUID:     userUUID,
				Name:     "test name",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "test-username",
			}
		)

		userRepository.On("GetOne", userUUID).Once().Return(user.User{}, domain.ErrNotExists)

		handler := NewGetProfileHandler(getprofile.NewUseCase(&userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			userUUID = "user-uuid"

			u = user.User{
				UUID:     userUUID,
				Name:     "test name",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "test-username",
			}
		)

		userRepository.On("GetOne", userUUID).Once().Return(user.User{}, errors.New("unexpected error"))

		handler := NewGetProfileHandler(getprofile.NewUseCase(&userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
