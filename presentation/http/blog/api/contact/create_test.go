package contact

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

	"github.com/khanzadimahdi/testproject/application/contact/createMessage"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("sends a message", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository
			requestValidator  validator.MockValidator
		)

		m := contact.Message{
			Subject: "a subject",
			Body:    "a body",
			Email:   "user@example.com",
			Phone:   "09123456789",
		}

		body := createMessage.Request{
			Subject: m.Subject,
			Body:    m.Body,
			Email:   m.Email,
			Phone:   m.Phone,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		requestValidator.On("Validate", &body).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		contactRepository.On("Save", mock.Anything, &m).Once().Return("message-uuid", nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewCreateHandler(createMessage.NewUseCase(&contactRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository
			requestValidator  validator.MockValidator
		)

		body := createMessage.Request{}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		requestValidator.On("Validate", &body).Once().Return(domain.ValidationErrors{
			"subject": "this field is required",
			"body":    "this field is required",
			"email":   "either an email address or a phone number is required",
			"phone":   "either an email address or a phone number is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewCreateHandler(createMessage.NewUseCase(&contactRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		contactRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/send-message-validation-errors.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository
			requestValidator  validator.MockValidator
		)

		m := contact.Message{
			Subject: "a subject",
			Body:    "a body",
			Email:   "user@example.com",
		}

		body := createMessage.Request{
			Subject: m.Subject,
			Body:    m.Body,
			Email:   m.Email,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		requestValidator.On("Validate", &body).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		contactRepository.On("Save", mock.Anything, &m).Once().Return("", errors.New("some unwanted error"))
		defer contactRepository.AssertExpectations(t)

		handler := NewCreateHandler(createMessage.NewUseCase(&contactRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
