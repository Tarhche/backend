package getUserComment

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
			userUUID    = "user-uuid"

			a = comment.Comment{
				UUID: commentUUID,
			}
			expectedResponse = Response{
				UUID:       commentUUID,
				ApprovedAt: a.ApprovedAt.Format(time.RFC3339),
				CreatedAt:  a.CreatedAt.Format(time.RFC3339),
			}
		)

		a.AuthorUUID = "author-uuid"
		expectedResponse.Author.UUID = a.AuthorUUID
		expectedResponse.Author.Username = "author-username"

		commentRepository.On("GetOneByAuthorUUID", commentUUID, userUUID).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", a.AuthorUUID).Once().Return(user.User{UUID: a.AuthorUUID, Username: "author-username"}, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(commentUUID, userUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting a comment fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			commentUUID = "role-uuid"
			userUUID    = "user-uuid"

			expectedError = errors.New("error")
		)

		commentRepository.On("GetOneByAuthorUUID", commentUUID, userUUID).Once().Return(comment.Comment{}, expectedError)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(commentUUID, userUUID)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("getting a comment's userinfo fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			commentUUID = "role-uuid"
			userUUID    = "user-uuid"

			a = comment.Comment{
				UUID: commentUUID,
			}

			expectedError = errors.New("error")
		)

		a.AuthorUUID = "author-uuid"

		commentRepository.On("GetOneByAuthorUUID", commentUUID, userUUID).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", a.AuthorUUID).Once().Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(commentUUID, userUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
