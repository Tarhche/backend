package getUserBookmarks

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/bookmarks"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("user's bookmarks", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				OwnerUUID: "owner-uuid",
			}

			b = []bookmark.Bookmark{
				{
					UUID:       "uuid-1",
					Title:      "title-1",
					ObjectUUID: "title-uuid-1",
					ObjectType: "article",
					OwnerUUID:  request.OwnerUUID,
					CreatedAt:  time.Now(),
				},
				{
					UUID:       "uuid-2",
					Title:      "title-2",
					ObjectUUID: "title-uuid-2",
					ObjectType: "article",
					OwnerUUID:  request.OwnerUUID,
					CreatedAt:  time.Now(),
				},
				{
					UUID:       "uuid-3",
					Title:      "title-3",
					ObjectUUID: "title-uui-3",
					ObjectType: "article",
					OwnerUUID:  request.OwnerUUID,
					CreatedAt:  time.Now(),
				},
			}

			expectedResponse = Response{
				Items: []bookmarkResponse{
					{
						Title:      b[0].Title,
						ObjectUUID: b[0].ObjectUUID,
						ObjectType: b[0].ObjectType,
						CreatedAt:  b[0].CreatedAt.Format(time.RFC3339),
					},
					{
						Title:      b[1].Title,
						ObjectUUID: b[1].ObjectUUID,
						ObjectType: b[1].ObjectType,
						CreatedAt:  b[1].CreatedAt.Format(time.RFC3339),
					},
					{
						Title:      b[2].Title,
						ObjectUUID: b[2].ObjectUUID,
						ObjectType: b[2].ObjectType,
						CreatedAt:  b[2].CreatedAt.Format(time.RFC3339),
					},
				},
				Pagination: pagination{
					TotalPages:  1,
					CurrentPage: 1,
				},
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("CountByOwnerUUID", request.OwnerUUID).Once().Return(uint(len(b)), nil)
		bookmarkRepository.On("GetAllByOwnerUUID", request.OwnerUUID, uint(0), uint(limit)).Once().Return(b, nil)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)
		assert.NoError(t, err)

		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"owner_uuid": "owner uuid is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "CountByOwnerUUID")
		bookmarkRepository.AssertNotCalled(t, "GetAllByOwnerUUID")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("no bookmarks", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				OwnerUUID: "owner-uuid",
			}

			expectedResponse = Response{
				Items: []bookmarkResponse{},
				Pagination: pagination{
					TotalPages:  0,
					CurrentPage: 1,
				},
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("CountByOwnerUUID", request.OwnerUUID).Once().Return(uint(0), nil)
		bookmarkRepository.On("GetAllByOwnerUUID", request.OwnerUUID, uint(0), uint(limit)).Once().Return(nil, nil)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on count", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				OwnerUUID: "owner-uuid",
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("CountByOwnerUUID", request.OwnerUUID).Once().Return(uint(0), expectedError)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		bookmarkRepository.AssertNotCalled(t, "GetAllByOwnerUUID")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, nil, response)
	})

	t.Run("error on getting bookmarks", func(t *testing.T) {
		t.Parallel()

		var (
			bookmarkRepository bookmarks.MockBookmarksRepository
			validator          validator.MockValidator

			request = Request{
				OwnerUUID: "owner-uuid",
			}

			expectedError = errors.New("some error")
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		bookmarkRepository.On("CountByOwnerUUID", request.OwnerUUID).Once().Return(uint(0), nil)
		bookmarkRepository.On("GetAllByOwnerUUID", request.OwnerUUID, uint(0), uint(limit)).Once().Return(nil, expectedError)
		defer bookmarkRepository.AssertExpectations(t)

		response, err := NewUseCase(&bookmarkRepository, &validator).Execute(&request)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, nil, response)
	})
}
