package getComment

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("gets a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			commentUUID = "role-uuid"
			a           = comment.Comment{
				UUID: commentUUID,
			}
			expectedResponse = Response{
				UUID:       commentUUID,
				CreatedAt:  a.CreatedAt.Format(time.RFC3339),
				ApprovedAt: a.ApprovedAt.Format(time.RFC3339),
			}
		)

		a.Author.UUID = "author-uuid"
		expectedResponse.Author.UUID = a.Author.UUID

		commentRepository.On("GetOne", commentUUID).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", a.Author.UUID).Once().Return(user.User{UUID: a.Author.UUID}, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(commentUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting a comment fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			commentUUID   = "comment-uuid"
			expectedError = errors.New("error")
		)

		commentRepository.On("GetOne", commentUUID).Once().Return(comment.Comment{}, expectedError)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(commentUUID)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("getting a comment's userinfo fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			commentUUID = "comment-uuid"
			a           = comment.Comment{
				UUID: commentUUID,
			}

			expectedError = errors.New("error")
		)

		a.Author.UUID = "author-uuid"

		commentRepository.On("GetOne", commentUUID).Once().Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", a.Author.UUID).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(commentUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
