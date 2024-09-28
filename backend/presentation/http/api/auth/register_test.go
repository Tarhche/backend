package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth/register"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
)

func TestRegisterHandler(t *testing.T) {
	mailFrom := "info@noreply.nowhere.loc"
	templateName := "resources/view/mail/auth/register"

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("refresh token", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			r = register.Request{
				Identity: "test@test-mail.test",
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, r.Identity, "Registration", mock.AnythingOfType("[]uint8")).Once().Return(nil)
		defer mailer.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer
		)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		expected, err := os.ReadFile("testdata/register-response-validation-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			r = register.Request{
				Identity: "test@test-mail.test",
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, errors.New("undexprected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewRegisterHandler(register.NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
