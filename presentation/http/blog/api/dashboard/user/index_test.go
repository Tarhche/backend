package user

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("show users", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

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

		userRepository.On("Count").Once().Return(uint(len(a)), nil)
		userRepository.On("GetAll", uint(0), uint(10)).Return(a, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewIndexHandler(getusers.NewUseCase(&userRepository))

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
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID: "user-uuid",
			}
		)

		userRepository.On("Count").Once().Return(uint(0), nil)
		userRepository.On("GetAll", uint(0), uint(10)).Return(nil, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewIndexHandler(getusers.NewUseCase(&userRepository))

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
}
