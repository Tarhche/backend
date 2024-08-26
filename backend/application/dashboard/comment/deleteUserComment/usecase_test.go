package deleteUserComment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("deletes a comment", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository

			r = Request{
				CommentUUID: "comment-uuid",
				UserUUID:    "user-uuid",
			}
		)

		commentRepository.On("DeleteByAuthorUUID", r.CommentUUID, r.UserUUID).Return(nil)
		defer commentRepository.AssertExpectations(t)

		err := NewUseCase(&commentRepository).Execute(r)

		assert.NoError(t, err)
	})

	t.Run("deleting the comment fails", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository

			r = Request{
				CommentUUID: "comment-uuid",
				UserUUID:    "user-uuid",
			}

			expectedError = errors.New("comment deletion failed")
		)

		commentRepository.On("DeleteByAuthorUUID", r.CommentUUID, r.UserUUID).Return(expectedError)
		defer commentRepository.AssertExpectations(t)

		err := NewUseCase(&commentRepository).Execute(r)

		assert.ErrorIs(t, err, expectedError)
	})
}
