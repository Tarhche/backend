package bookmark

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/bookmark/getUserBookmarks"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestIndexUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("user's bookmarks", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			requestValidator   validator.MockValidator

			r = getUserBookmarks.Request{
				OwnerUUID: "owner-uuid",
				Page:      1,
			}

			u = user.User{
				UUID: r.OwnerUUID,
			}

			createdAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			b = []bookmark.Bookmark{
				{
					UUID:       "uuid-1",
					Title:      "title-1",
					ObjectUUID: "title-uuid-1",
					ObjectType: "article",
					OwnerUUID:  r.OwnerUUID,
					CreatedAt:  createdAt,
				},
				{
					UUID:       "uuid-2",
					Title:      "title-2",
					ObjectUUID: "title-uuid-2",
					ObjectType: "article",
					OwnerUUID:  r.OwnerUUID,
					CreatedAt:  createdAt,
				},
				{
					UUID:       "uuid-3",
					Title:      "title-3",
					ObjectUUID: "title-uui-3",
					ObjectType: "article",
					OwnerUUID:  r.OwnerUUID,
					CreatedAt:  createdAt,
				},
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		bookmarkRepository.On("CountByOwnerUUID", r.OwnerUUID).Once().Return(uint(len(b)), nil)
		bookmarkRepository.On("GetAllByOwnerUUID", r.OwnerUUID, uint(0), uint(10)).Once().Return(b, nil)
		defer bookmarkRepository.AssertExpectations(t)

		handler := NewIndexUserBookmarksHandler(getUserBookmarks.NewUseCase(&bookmarkRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-bookmarks-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			requestValidator   validator.MockValidator

			r = getUserBookmarks.Request{
				OwnerUUID: "owner-uuid",
				Page:      1,
			}

			u = user.User{
				UUID: r.OwnerUUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		bookmarkRepository.On("CountByOwnerUUID", r.OwnerUUID).Once().Return(uint(0), nil)
		bookmarkRepository.On("GetAllByOwnerUUID", r.OwnerUUID, uint(0), uint(10)).Once().Return(nil, nil)
		defer bookmarkRepository.AssertExpectations(t)

		handler := NewIndexUserBookmarksHandler(getUserBookmarks.NewUseCase(&bookmarkRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-bookmarks-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})
}
