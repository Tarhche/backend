package bookmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/bookmark/deleteUserBookmark"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
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
			authorizer         domain.MockAuthorizer
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

		authorizer.On("Authorize", u.UUID, permission.SelfBookmarksDelete).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		bookmarkRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Return(nil)
		defer bookmarkRepository.AssertExpectations(t)

		handler := NewDeleteUserBookmarkHandler(deleteUserBookmark.NewUseCase(&bookmarkRepository, &requestValidator), &authorizer)

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

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			authorizer         domain.MockAuthorizer
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

		authorizer.On("Authorize", u.UUID, permission.SelfBookmarksDelete).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteUserBookmarkHandler(deleteUserBookmark.NewUseCase(&bookmarkRepository, &requestValidator), &authorizer)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodDelete, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			authorizer         domain.MockAuthorizer
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

		authorizer.On("Authorize", u.UUID, permission.SelfBookmarksDelete).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteUserBookmarkHandler(deleteUserBookmark.NewUseCase(&bookmarkRepository, &requestValidator), &authorizer)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodDelete, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
