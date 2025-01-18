package deleteUserBookmark

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deleting user's bookmark succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r = Request{
				ObjectType: "article",
				ObjectUUID: "article-uuid",
				OwnerUUID:  "user-uuid",
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Return(nil)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"object_type": "object type is not supported",
					"object_uuid": "object uuid is required",
					"owner_uuid":  "owner uuid is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&r)

		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")

		assert.Nil(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("deleting user's bookmark fails", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r = Request{
				ObjectType: "article",
				ObjectUUID: "article-uuid",
				OwnerUUID:  "user-uuid",
			}

			expectedErr = errors.New("undexpected error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Return(expectedErr)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
