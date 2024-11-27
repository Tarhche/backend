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
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth/forgetpassword"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestForgetPasswordHandler(t *testing.T) {
	t.Parallel()

	t.Run("sends forget password email", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r       = forgetpassword.Request{Identity: "something@somewhere.loc"}
			command = forgetpassword.SendForgetPasswordEmail{Identity: r.Identity}
			u       = user.User{
				UUID:  "user-uuid",
				Email: r.Identity,
			}
		)

		commandPayload, err := json.Marshal(command)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		asyncCommandBus.On("Publish", context.Background(), forgetpassword.SendForgetPasswordEmailName, commandPayload).Return(nil)
		defer asyncCommandBus.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

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

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock
		)

		requestValidator.On("Validate", &forgetpassword.Request{}).Once().Return(domain.ValidationErrors{
			"identity": "identity is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		asyncCommandBus.AssertNotCalled(t, "Publish")

		expected, err := os.ReadFile("testdata/forgetpassword-response-validation-fail.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = forgetpassword.Request{Identity: "something@somewhere.loc"}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translator.On("Translate", "identity (email/username) not exists", mock2.Anything).Once().Return("identity (email/username) not exists")
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

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
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			asyncCommandBus  mock.MockPublishSubscriber
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			r = forgetpassword.Request{Identity: "something@somewhere.loc"}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("some error"))
		defer userRepository.AssertExpectations(t)

		handler := NewForgetPasswordHandler(forgetpassword.NewUseCase(&userRepository, &asyncCommandBus, &translator, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		asyncCommandBus.AssertNotCalled(t, "Publish")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
