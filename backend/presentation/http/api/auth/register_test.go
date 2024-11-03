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

	"github.com/khanzadimahdi/testproject/application/auth/register"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestRegisterHandler(t *testing.T) {
	t.Run("refresh token", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = register.Request{
				Identity: "test@test-mail.test",
			}

			command = register.SendRegistrationEmail{
				Identity: r.Identity,
			}
		)

		commandPayload, err := json.Marshal(command)
		assert.NoError(t, err)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), register.SendRegisterationEmailName, commandPayload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, &asyncCommandBus))

		var payload bytes.Buffer
		err = json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber
		)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, &asyncCommandBus))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		expected, err := os.ReadFile("testdata/register-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository  users.MockUsersRepository
			asyncCommandBus mock.MockPublishSubscriber

			r = register.Request{
				Identity: "test@test-mail.test",
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("undexprected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, &asyncCommandBus))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
