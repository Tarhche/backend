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
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestRegisterHandler(t *testing.T) {
	t.Parallel()

	t.Run("refresh token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = register.Request{
				Identity: "test@test-mail.test",
			}

			command = register.SendRegistrationEmail{
				Identity: r.Identity,
			}
		)

		commandPayload, err := json.Marshal(command)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), register.SendRegisterationEmailName, commandPayload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

		var payload bytes.Buffer
		err = json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock
		)

		requestValidator.On("Validate", &register.Request{}).Once().Return(domain.ValidationErrors{
			"identity": "identity is not a valid email address",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		expected, err := os.ReadFile("testdata/register-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = register.Request{
				Identity: "test@test-mail.test",
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("undexprected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
