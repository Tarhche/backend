package user

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getuser "github.com/khanzadimahdi/testproject/application/dashboard/user/getUser"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID: "user-uuid",
			}

			userUUID = "user-uuid"
			a        = user.User{
				UUID: userUUID,
			}
		)

		userRepository.On("GetOne", userUUID).Return(a, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewShowHandler(getuser.NewUseCase(&userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", a.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-a-user-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID: "user-uuid",
			}

			userUUID = "user-uuid"
			a        = user.User{
				UUID: userUUID,
			}
		)

		userRepository.On("GetOne", userUUID).Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		handler := NewShowHandler(getuser.NewUseCase(&userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", a.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
