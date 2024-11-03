package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth/forgetpassword"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestForgetPasswordHandler(t *testing.T) {
	t.Run("sends forget password email", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r       = forgetpassword.Request{Identity: "something@somewhere.loc"}
			command = forgetpassword.SendForgetPasswordEmail{Identity: r.Identity}
			u       = user.User{
				UUID:  "user-uuid",
				Email: r.Identity,
			}
		)

		commandPayload, err := json.Marshal(command)
		assert.NoError(t, err)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), forgetpassword.SendForgetPasswordEmailName, commandPayload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus))

		var payload bytes.Buffer
		err = json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
		)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		expected, err := os.ReadFile("testdata/forgetpassword-response-validation-fail.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("user not found", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = forgetpassword.Request{Identity: "something@somewhere.loc"}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		expected, err := os.ReadFile("testdata/forgetpassword-response-user-not-found.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = forgetpassword.Request{Identity: "something@somewhere.loc"}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("some error"))
		defer userRepository.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		userRepository.AssertNotCalled(t, "GetOneByIdentity")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
