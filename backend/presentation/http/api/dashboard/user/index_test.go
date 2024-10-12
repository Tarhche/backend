package user

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestIndexHandler(t *testing.T) {
	t.Run("show users", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}

			a = []user.User{
				{
					UUID:     "article-uuid-1",
					Name:     "John Doe",
					Email:    "johndoe@test.com",
					Username: "john.doe",
					PasswordHash: password.Hash{
						Value: make([]byte, 10),
						Salt:  make([]byte, 20),
					},
				},
				{
					UUID:     "article-uuid-2",
					Avatar:   "random-avatar",
					Username: "test-username",
				},
				{
					UUID: "article-uuid-3",
					Name: "test name",
				},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		userRepository.On("Count").Once().Return(uint(len(a)), nil)
		userRepository.On("GetAll", uint(0), uint(10)).Return(a, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewIndexHandler(getusers.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-users-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		userRepository.On("Count").Once().Return(uint(0), nil)
		userRepository.On("GetAll", uint(0), uint(10)).Return(nil, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewIndexHandler(getusers.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-users-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorised", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersIndex).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getusers.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Count")
		userRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			authorizer     domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.UsersIndex).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getusers.NewUseCase(&userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "Count")
		userRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.String(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
