package user

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/stretchr/testify/assert"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}

			r = deleteuser.Request{UserUUID: "user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersDelete).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		userRepository.On("Delete", r.UserUUID).Return(nil)
		defer userRepository.AssertExpectations(t)
		handler := NewDeleteHandler(deleteuser.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.UserUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("unauthorised", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}

			r = deleteuser.Request{UserUUID: "user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersDelete).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deleteuser.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.UserUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}

			r = deleteuser.Request{UserUUID: "user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersDelete).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deleteuser.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.UserUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
