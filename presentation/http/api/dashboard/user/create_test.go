package user

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("create user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           mock.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
			}

			r = createuser.Request{
				Name:     "test name",
				Email:    "test@test.com",
				Username: "test-username",
				Password: "test",
			}

			userUUID = "test-user-uuid"
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", r.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock2.Anything).Once().Return(userUUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock2.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"))
		defer hasher.AssertExpectations(t)

		handler := NewCreateHandler(createuser.NewUseCase(&userRepository, &hasher, &requestValidator, &translator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		expectedBody, err := os.ReadFile("testdata/create-users-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			hasher           mock.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
			}

			validationErrors = domain.ValidationErrors{
				"email":    "email is required",
				"name":     "name is required",
				"password": "password is required",
			}
		)

		requestValidator.On("Validate", &createuser.Request{}).Once().Return(validationErrors)
		defer requestValidator.AssertExpectations(t)

		handler := NewCreateHandler(createuser.NewUseCase(&userRepository, &hasher, &requestValidator, &translator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		expectedBody, err := os.ReadFile("testdata/create-users-validation-failed-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
