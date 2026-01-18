package user

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID: "user-uuid",
			}

			r = deleteuser.Request{UserUUID: "user-uuid"}
		)

		userRepository.On("Delete", r.UserUUID).Return(nil)
		defer userRepository.AssertExpectations(t)
		handler := NewDeleteHandler(deleteuser.NewUseCase(&userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.UserUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
