package deleteUserBookmark

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("deleting user's bookmark succeeds", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository

			r = Request{
				ObjectType: "article",
				ObjectUUID: "article-uuid",
				OwnerUUID:  "user-uuid",
			}
		)

		bookmarkRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Return(nil)
		defer bookmarkRepository.AssertExpectations(t)

		err := NewUseCase(&bookmarkRepository).Execute(&r)

		assert.NoError(t, err)
	})

	t.Run("deleting user's bookmark fails", func(t *testing.T) {
		var (
			bookmarkRepository bookmarks.MockBookmarksRepository

			r = Request{
				ObjectType: "article",
				ObjectUUID: "article-uuid",
				OwnerUUID:  "user-uuid",
			}

			expectedErr = errors.New("undexpected error")
		)

		bookmarkRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Return(expectedErr)
		defer bookmarkRepository.AssertExpectations(t)

		err := NewUseCase(&bookmarkRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
	})
}
