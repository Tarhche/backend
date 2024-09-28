package bookmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/bookmark/bookmarkExists"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
)

func TestExistsHandler(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
		)

		u := user.User{
			UUID: "user-uuid-1",
		}

		b := bookmark.Bookmark{
			UUID:       "bookmark-uuid",
			Title:      "bookmark-title",
			ObjectUUID: "object-uuid",
			ObjectType: "article",
			OwnerUUID:  u.UUID,
			CreatedAt:  time.Now(),
		}

		bookmarkRepository.On("GetByOwnerUUID", b.OwnerUUID, b.ObjectType, b.ObjectUUID).Once().Return(b, nil)
		defer bookmarkRepository.AssertExpectations(t)

		handler := NewExistsHandler(bookmarkExists.NewUseCase(&bookmarkRepository))

		r := bookmarkExists.Request{
			ObjectType: b.ObjectType,
			ObjectUUID: b.ObjectUUID,
		}

		var paylaod bytes.Buffer

		err := json.NewEncoder(&paylaod).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &paylaod)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/bookmark-exists-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
		)

		u := user.User{
			UUID: "user-uuid-1",
		}

		handler := NewExistsHandler(bookmarkExists.NewUseCase(&bookmarkRepository))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		bookmarkRepository.AssertNotCalled(t, "GetByOwnerUUID")

		expected, err := os.ReadFile("testdata/bookmark-exists-validation-response.json")
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
			UUID:       "bookmark-uuid",
			Title:      "bookmark-title",
			ObjectUUID: "object-uuid",
			ObjectType: "article",
			OwnerUUID:  u.UUID,
			CreatedAt:  time.Now(),
		}

		bookmarkRepository.On("GetByOwnerUUID", b.OwnerUUID, b.ObjectType, b.ObjectUUID).Once().Return(bookmark.Bookmark{}, errors.New("something went wrong"))
		defer bookmarkRepository.AssertExpectations(t)

		handler := NewExistsHandler(bookmarkExists.NewUseCase(&bookmarkRepository))

		r := bookmarkExists.Request{
			ObjectType: b.ObjectType,
			ObjectUUID: b.ObjectUUID,
		}

		var paylaod bytes.Buffer

		err := json.NewEncoder(&paylaod).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &paylaod)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
