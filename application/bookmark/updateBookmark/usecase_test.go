package updateBookmark

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

	t.Run("removes a bookmark", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				Keep:       false,
				Title:      "test",
				OwnerUUID:  "owner-uuid",
				ObjectType: "article",
				ObjectUUID: "object-uuid",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("DeleteByOwnerUUID", request.OwnerUUID, request.ObjectType, request.ObjectUUID).Once().Return(nil)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("saves a bookmark", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				Keep:       true,
				Title:      "test",
				OwnerUUID:  "owner-uuid",
				ObjectType: "article",
				ObjectUUID: "object-uuid",
			}

			b = bookmark.Bookmark{
				Title:      request.Title,
				ObjectUUID: request.ObjectUUID,
				ObjectType: request.ObjectType,
				OwnerUUID:  request.OwnerUUID,
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("Save", &b).Once().Return("test-uuid", nil)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation failure", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request Request

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"title":       "title is required",
					"object_type": "object type is not supported",
					"object_uuid": "object uuid is required",
					"owner_uuid":  "owner uuid is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")
		bookmarkRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on removing bookmark", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				Keep:       false,
				Title:      "test",
				OwnerUUID:  "owner-uuid",
				ObjectType: "article",
				ObjectUUID: "object-uuid",
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("DeleteByOwnerUUID", request.OwnerUUID, request.ObjectType, request.ObjectUUID).Once().Return(expectedErr)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("error on saving bookmark", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				Keep:       true,
				Title:      "test",
				OwnerUUID:  "owner-uuid",
				ObjectType: "article",
				ObjectUUID: "object-uuid",
			}

			b = bookmark.Bookmark{
				Title:      request.Title,
				ObjectUUID: request.ObjectUUID,
				ObjectType: request.ObjectType,
				OwnerUUID:  request.OwnerUUID,
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("Save", &b).Once().Return("", expectedErr)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "DeleteByOwnerUUID")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
