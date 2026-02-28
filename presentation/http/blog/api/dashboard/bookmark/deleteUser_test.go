package bookmark

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/bookmark/deleteUserBookmark"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestDeleteUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete user's bookmark", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			requestValidator   validator.MockValidator

			u = user.User{
				UUID: "user-uuid",
			}

			r = deleteUserBookmark.Request{
				ObjectType: "article",
				ObjectUUID: "article-uuid",
				OwnerUUID:  u.UUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		bookmarkRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Return(nil)
		defer bookmarkRepository.AssertExpectations(t)

		handler := NewDeleteUserBookmarkHandler(deleteUserBookmark.NewUseCase(&bookmarkRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodDelete, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
