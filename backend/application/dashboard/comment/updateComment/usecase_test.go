package updateComment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("updates a comment", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository

			r = Request{
				UUID:       "comment-uuid",
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				ObjectUUID: "test-object-uuid",
				ObjectType: "article",
			}

			c = comment.Comment{
				UUID: r.UUID,
				Body: r.Body,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				ObjectUUID: r.ObjectUUID,
				ObjectType: r.ObjectType,
			}
		)

		commentRepository.On("Save", &c).Once().Return(r.UUID, nil)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository).Execute(r)
		assert.NoError(t, err)
		assert.Equal(t, &Response{}, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			r                 = Request{}
			expectedResponse  = Response{
				ValidationErrors: validationErrors{
					"uuid":        "uuid is required",
					"body":        "body is required",
					"object_type": "object type is not supported",
					"object_uuid": "object_uuid is required",
				},
			}
		)

		response, err := NewUseCase(&commentRepository).Execute(r)

		commentRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("updating a comment fails", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			r                 = Request{
				UUID:       "comment-uuid",
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				ObjectUUID: "test-object-uuid",
				ObjectType: "article",
			}

			c = comment.Comment{
				UUID: r.UUID,
				Body: r.Body,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				ObjectUUID: r.ObjectUUID,
				ObjectType: r.ObjectType,
			}

			expectedErr = errors.New("error happened")
		)

		commentRepository.On("Save", &c).Once().Return("", expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository).Execute(r)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
