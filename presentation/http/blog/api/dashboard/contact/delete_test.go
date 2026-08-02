package contact

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/contact/deleteMessage"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete a message", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			u = user.User{UUID: "auth-user-uuid"}
		)

		contactRepository.On("Delete", mock.Anything, "message-uuid").Once().Return(nil)
		defer contactRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deleteMessage.NewUseCase(&contactRepository))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", "message-uuid")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
