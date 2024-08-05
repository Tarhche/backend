package getComments

import (
	"errors"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("get comments", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			r = Request{
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			c = []comment.Comment{
				{UUID: "comment-uuid-01", Author: author.Author{UUID: "user-uuid-01"}},
				{UUID: "comment-uuid-02", Author: author.Author{UUID: "user-uuid-02"}},
				{UUID: "comment-uuid-03", Author: author.Author{UUID: "user-uuid-same"}},
				{UUID: "comment-uuid-04", Author: author.Author{UUID: "user-uuid-same"}},
			}
			authorUUIDs = []string{
				"user-uuid-01",
				"user-uuid-02",
				"user-uuid-same",
				"user-uuid-same",
			}
			u = []user.User{
				{UUID: "user-uuid-01"},
				{UUID: "user-uuid-02"},
				{UUID: "user-uuid-same"},
			}
		)

		commentRepository.On("CountApprovedByObjectUUID", r.ObjectType, r.ObjectUUID).Once().Return(uint(len(c)), nil)
		commentRepository.On("GetApprovedByObjectUUID", r.ObjectType, r.ObjectUUID, uint(0), uint(10)).Once().Return(c, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", authorUUIDs).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("error on getting total count of approved comments", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			r = Request{
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			expectedErr = errors.New("test error")
		)

		commentRepository.On("CountApprovedByObjectUUID", r.ObjectType, r.ObjectUUID).Once().Return(uint(0), expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		commentRepository.AssertNotCalled(t, "GetApprovedByObjectUUID")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

	t.Run("error on getting approved comments", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			r = Request{
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			approvedCommentsCount = uint(10)
			expectedErr           = errors.New("test error")
		)

		commentRepository.On("CountApprovedByObjectUUID", r.ObjectType, r.ObjectUUID).Once().Return(approvedCommentsCount, nil)
		commentRepository.On("GetApprovedByObjectUUID", r.ObjectType, r.ObjectUUID, uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

	t.Run("error on getting comments' author", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			r = Request{
				ObjectType: "article",
				ObjectUUID: "test-uuid",
			}

			c = []comment.Comment{
				{UUID: "comment-uuid-01", Author: author.Author{UUID: "user-uuid-01"}},
				{UUID: "comment-uuid-02", Author: author.Author{UUID: "user-uuid-02"}},
				{UUID: "comment-uuid-03", Author: author.Author{UUID: "user-uuid-same"}},
				{UUID: "comment-uuid-04", Author: author.Author{UUID: "user-uuid-same"}},
			}
			authorUUIDs = []string{
				"user-uuid-01",
				"user-uuid-02",
				"user-uuid-same",
				"user-uuid-same",
			}

			expectedErr = errors.New("test error")
		)

		commentRepository.On("CountApprovedByObjectUUID", r.ObjectType, r.ObjectUUID).Once().Return(uint(len(c)), nil)
		commentRepository.On("GetApprovedByObjectUUID", r.ObjectType, r.ObjectUUID, uint(0), uint(10)).Once().Return(c, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", authorUUIDs).Once().Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})
}
