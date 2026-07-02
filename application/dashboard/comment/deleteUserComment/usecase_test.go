package deleteUserComment

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository

			r = Request{
				CommentUUID: "comment-uuid",
				UserUUID:    "user-uuid",
			}
		)

		commentRepository.On("DeleteByAuthorUUID", mock.Anything, r.CommentUUID, r.UserUUID).Return(nil)
		defer commentRepository.AssertExpectations(t)

		err := NewUseCase(&commentRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting the comment fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository

			r = Request{
				CommentUUID: "comment-uuid",
				UserUUID:    "user-uuid",
			}

			expectedError = errors.New("comment deletion failed")
		)

		commentRepository.On("DeleteByAuthorUUID", mock.Anything, r.CommentUUID, r.UserUUID).Return(expectedError)
		defer commentRepository.AssertExpectations(t)

		err := NewUseCase(&commentRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
