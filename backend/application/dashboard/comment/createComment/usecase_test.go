package createComment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("creates a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			validator         validator.MockValidator

			r = Request{
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				ObjectUUID: "test-object-uuid",
				ObjectType: "article",
			}

			c = comment.Comment{
				Body: r.Body,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				ObjectUUID: r.ObjectUUID,
				ObjectType: r.ObjectType,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		commentRepository.On("Save", &c).Once().Return("comment-uuid", nil)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			validator         validator.MockValidator

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"body":        "body is required",
					"object_type": "object type is not supported",
					"object_uuid": "object_uuid is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &validator).Execute(&r)

		commentRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("saving the comment fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			validator         validator.MockValidator

			r = Request{
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				ObjectUUID: "test-object-uuid",
				ObjectType: "article",
			}

			c = comment.Comment{
				Body: r.Body,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				ObjectUUID: r.ObjectUUID,
				ObjectType: r.ObjectType,
			}

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		commentRepository.On("Save", &c).Once().Return("", expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
