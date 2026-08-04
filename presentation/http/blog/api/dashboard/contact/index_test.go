package contact

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/contact/getMessages"
	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("show messages", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}

			createdAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			m = []contact.Message{
				{
					UUID:      "message-uuid-1",
					Subject:   "subject-1",
					Body:      "body-1",
					Email:     "user@example.com",
					CreatedAt: createdAt,
				},
				{
					UUID:      "message-uuid-2",
					Subject:   "subject-2",
					Body:      "body-2",
					Phone:     "09123456789",
					ReadAt:    createdAt,
					CreatedAt: createdAt,
				},
			}
		)

		contactRepository.On("Count", mock.Anything).Once().Return(uint(len(m)), nil)
		contactRepository.On("GetAll", mock.Anything, uint(0), uint(10)).Once().Return(m, nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewIndexHandler(getMessages.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?page=%d", 1), nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-messages-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}
		)

		contactRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)
		contactRepository.On("GetAll", mock.Anything, uint(0), uint(10)).Once().Return(nil, nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewIndexHandler(getMessages.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?page=%d", 1), nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-messages-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})
}
