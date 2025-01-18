package updateUserComment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("updates a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			validator         validator.MockValidator

			r = Request{
				UUID:     "comment-uuid",
				Body:     "test body",
				UserUUID: "user-uuid",
			}

			c = comment.Comment{
				UUID: r.UUID,
				Body: r.Body,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		commentRepository.On("GetOneByAuthorUUID", r.UUID, r.UserUUID).Once().Return(c, nil)
		commentRepository.On("Save", &c).Once().Return(r.UUID, nil)
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
					"uuid":      "uuid is required",
					"body":      "body is required",
					"user_uuid": "user's uuid is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &validator).Execute(&r)

		commentRepository.AssertNotCalled(t, "GetOneByAuthorUUID")
		commentRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get one by user uuid fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			validator         validator.MockValidator

			r = Request{
				UUID:     "comment-uuid",
				Body:     "test body",
				UserUUID: "user-uuid",
			}

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		commentRepository.On("GetOneByAuthorUUID", r.UUID, r.UserUUID).Once().Return(comment.Comment{}, expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &validator).Execute(&r)

		commentRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("updating a comment fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			validator         validator.MockValidator

			r = Request{
				UUID:     "comment-uuid",
				Body:     "test body",
				UserUUID: "user-uuid",
			}

			c = comment.Comment{
				UUID: r.UUID,
				Body: r.Body,
			}

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		commentRepository.On("GetOneByAuthorUUID", r.UUID, r.UserUUID).Once().Return(c, nil)
		commentRepository.On("Save", &c).Once().Return("", expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &validator).Execute(&r)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
