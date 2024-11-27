package bookmarkExists

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("bookmark exists", func(t *testing.T) {
		t.Parallel()

		var (
			boomkarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r = Request{
				OwnerUUID:  "test-user-uuid",
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			b bookmark.Bookmark
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		boomkarkRepository.On("GetByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Once().Return(b, nil)
		defer boomkarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&boomkarkRepository, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
		assert.True(t, response.Exist)
	})

	t.Run("bookmark not exists", func(t *testing.T) {
		t.Parallel()

		var (
			boomkarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r = Request{
				OwnerUUID:  "test-user-uuid",
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			b bookmark.Bookmark
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		boomkarkRepository.On("GetByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Once().Return(b, domain.ErrNotExists)
		defer boomkarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&boomkarkRepository, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
		assert.False(t, response.Exist)
	})

	t.Run("validation failure", func(t *testing.T) {
		t.Parallel()

		var (
			boomkarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r Request

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

		response, err := NewUseCase(&boomkarkRepository, &validator).Execute(&r)

		boomkarkRepository.AssertNotCalled(t, "GetByOwnerUUID")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on getting bookmark details", func(t *testing.T) {
		t.Parallel()

		var (
			boomkarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			r = Request{
				OwnerUUID:  "test-user-uuid",
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			expectedErr = errors.New("some error")

			b bookmark.Bookmark
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		boomkarkRepository.On("GetByOwnerUUID", r.OwnerUUID, r.ObjectType, r.ObjectUUID).Once().Return(b, expectedErr)
		defer boomkarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&boomkarkRepository, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
