package createComment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("creates a comment", func(t *testing.T) {
		var (
			c comments.MockCommentsRepository
			r = Request{
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				ParentUUID: "test-parent-uuid",
				ObjectUUID: "test-object-uuid",
				ObjectType: "article",
			}

			cm = comment.Comment{
				Body: r.Body,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				ParentUUID: r.ParentUUID,
				ObjectUUID: r.ObjectUUID,
				ObjectType: r.ObjectType,
			}
		)

		c.On("Save", &cm).Once().Return("comment-uuid", nil)
		defer c.AssertExpectations(t)

		response, err := NewUseCase(&c).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			c                comments.MockCommentsRepository
			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"body":        "body is required",
					"object_type": "object type is not supported",
					"object_uuid": "object_uuid is required",
				},
			}
		)

		response, err := NewUseCase(&c).Execute(r)

		c.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response, "response does not match")
	})

	t.Run("failure on saving a comment", func(t *testing.T) {
		var (
			c comments.MockCommentsRepository
			r = Request{
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				ParentUUID: "test-parent-uuid",
				ObjectUUID: "test-object-uuid",
				ObjectType: "article",
			}

			cm = comment.Comment{
				Body: r.Body,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				ParentUUID: r.ParentUUID,
				ObjectUUID: r.ObjectUUID,
				ObjectType: r.ObjectType,
			}

			expectedErr = errors.New("save comment error")
		)

		c.On("Save", &cm).Once().Return("", expectedErr)
		defer c.AssertExpectations(t)

		response, err := NewUseCase(&c).Execute(r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
