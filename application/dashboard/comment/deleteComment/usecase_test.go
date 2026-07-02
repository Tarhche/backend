package deleteComment

import (
	"context"
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository

			r = Request{CommentUUID: "comment-uuid"}
		)

		commentRepository.On("Delete", mock.Anything, r.CommentUUID).Return(nil)
		defer commentRepository.AssertExpectations(t)

		err := NewUseCase(&commentRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting the comment fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository

			r             = Request{CommentUUID: "comment-uuid"}
			expectedError = errors.New("comment deletion failed")
		)

		commentRepository.On("Delete", mock.Anything, r.CommentUUID).Return(expectedError)
		defer commentRepository.AssertExpectations(t)

		err := NewUseCase(&commentRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
