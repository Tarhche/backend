package contact

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/contact/getMessage"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show a message", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}

			createdAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			m = contact.Message{
				UUID:      "message-uuid",
				Subject:   "a subject",
				Body:      "a body",
				Email:     "user@example.com",
				Phone:     "09123456789",
				CreatedAt: createdAt,
			}
		)

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewShowHandler(getMessage.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", m.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-a-message-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("message does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}
		)

		contactRepository.On("GetOne", mock.Anything, "message-uuid").Once().Return(contact.Message{}, domain.ErrNotExists)
		defer contactRepository.AssertExpectations(t)

		handler := NewShowHandler(getMessage.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", "message-uuid")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
