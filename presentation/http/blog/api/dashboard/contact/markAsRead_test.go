package contact

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/contact/markAsRead"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestMarkAsReadHandler(t *testing.T) {
	t.Parallel()

	t.Run("marks a message as read", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}

			m = contact.Message{
				UUID:    "message-uuid",
				Subject: "a subject",
				Body:    "a body",
			}
		)

		var payload bytes.Buffer
		assert.NoError(t, json.NewEncoder(&payload).Encode(markAsRead.Request{Read: true}))

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		contactRepository.On("Save", mock.Anything, mock.MatchedBy(func(saved *contact.Message) bool {
			return saved.IsRead()
		})).Once().Return(m.UUID, nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewMarkAsReadHandler(markAsRead.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", m.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		var body markAsRead.Response
		assert.NoError(t, json.NewDecoder(response.Body).Decode(&body))

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.Equal(t, m.UUID, body.UUID)
		assert.NotEqual(t, time.Time{}.Format(time.RFC3339), body.ReadAt)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("marks a message as unread", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}

			m = contact.Message{
				UUID:   "message-uuid",
				ReadAt: time.Now(),
			}
		)

		var payload bytes.Buffer
		assert.NoError(t, json.NewEncoder(&payload).Encode(markAsRead.Request{Read: false}))

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		contactRepository.On("Save", mock.Anything, mock.MatchedBy(func(saved *contact.Message) bool {
			return !saved.IsRead()
		})).Once().Return(m.UUID, nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewMarkAsReadHandler(markAsRead.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", m.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		var body markAsRead.Response
		assert.NoError(t, json.NewDecoder(response.Body).Decode(&body))

		assert.Equal(t, time.Time{}.Format(time.RFC3339), body.ReadAt)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("message does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}
		)

		var payload bytes.Buffer
		assert.NoError(t, json.NewEncoder(&payload).Encode(markAsRead.Request{Read: true}))

		contactRepository.On("GetOne", mock.Anything, "message-uuid").Once().Return(contact.Message{}, domain.ErrNotExists)
		defer contactRepository.AssertExpectations(t)

		handler := NewMarkAsReadHandler(markAsRead.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", "message-uuid")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		contactRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
