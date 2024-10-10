package bookmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/bookmark/updateBookmark"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
)

func TestUpdateHandler(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
		)

		u := user.User{
			UUID: "user-uuid-1",
		}

		b := bookmark.Bookmark{
			Title:      "test title",
			ObjectUUID: "object-uuid-1",
			ObjectType: "article",
			OwnerUUID:  u.UUID,
		}

		bookmarkRepository.On("DeleteByOwnerUUID", b.OwnerUUID, b.ObjectType, b.ObjectUUID).Once().Return(nil)
		defer bookmarkRepository.AssertExpectations(t)

		r := updateBookmark.Request{
			Keep:       false,
			Title:      b.Title,
			ObjectType: b.ObjectType,
			ObjectUUID: b.ObjectUUID,
		}

		var payload bytes.Buffer

		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler := NewUpdateHandler(updateBookmark.NewUseCase(&bookmarkRepository))

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
		)

		u := user.User{
			UUID: "user-uuid-1",
		}

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler := NewUpdateHandler(updateBookmark.NewUseCase(&bookmarkRepository))

		handler.ServeHTTP(response, request)

		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")

		expected, err := os.ReadFile("testdata/bookmark-update-validation-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
		)

		u := user.User{
			UUID: "user-uuid-1",
		}

		b := bookmark.Bookmark{
			Title:      "test title",
			ObjectUUID: "object-uuid-1",
			ObjectType: "article",
			OwnerUUID:  u.UUID,
		}

		bookmarkRepository.On("DeleteByOwnerUUID", b.OwnerUUID, b.ObjectType, b.ObjectUUID).Once().Return(errors.New("something wrong has happened"))
		defer bookmarkRepository.AssertExpectations(t)

		r := updateBookmark.Request{
			Keep:       false,
			Title:      b.Title,
			ObjectType: b.ObjectType,
			ObjectUUID: b.ObjectUUID,
		}

		var payload bytes.Buffer

		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler := NewUpdateHandler(updateBookmark.NewUseCase(&bookmarkRepository))

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
