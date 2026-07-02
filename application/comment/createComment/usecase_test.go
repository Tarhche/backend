package createComment

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("creates a comment", func(t *testing.T) {
		t.Parallel()

		var (
			c         comments.MockCommentsRepository
			validator validator.MockValidator

			r = Request{
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				ParentUUID:   "test-parent-uuid",
				ObjectUUID:   "test-object-uuid",
				ObjectType:   "article",
				LanguageCode: "en",
			}

			cm = comment.Comment{
				Body:         r.Body,
				AuthorUUID:   r.AuthorUUID,
				ParentUUID:   r.ParentUUID,
				ObjectUUID:   r.ObjectUUID,
				ObjectType:   r.ObjectType,
				LanguageCode: r.LanguageCode,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		c.On("Save", mock.Anything, &cm).Once().Return("comment-uuid", nil)
		defer c.AssertExpectations(t)

		response, err := NewUseCase(&c, &validator).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			c         comments.MockCommentsRepository
			validator validator.MockValidator

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"body":        "body is required",
					"object_type": "object type is not supported",
					"object_uuid": "object_uuid is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&c, &validator).Execute(context.Background(), &r)

		c.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response, "response does not match")
	})

	t.Run("failure on saving a comment", func(t *testing.T) {
		t.Parallel()

		var (
			c         comments.MockCommentsRepository
			validator validator.MockValidator

			r = Request{
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				ParentUUID:   "test-parent-uuid",
				ObjectUUID:   "test-object-uuid",
				ObjectType:   "article",
				LanguageCode: "en",
			}

			cm = comment.Comment{
				Body:         r.Body,
				AuthorUUID:   r.AuthorUUID,
				ParentUUID:   r.ParentUUID,
				ObjectUUID:   r.ObjectUUID,
				ObjectType:   r.ObjectType,
				LanguageCode: r.LanguageCode,
			}

			expectedErr = errors.New("save comment error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		c.On("Save", mock.Anything, &cm).Once().Return("", expectedErr)
		defer c.AssertExpectations(t)

		response, err := NewUseCase(&c, &validator).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
