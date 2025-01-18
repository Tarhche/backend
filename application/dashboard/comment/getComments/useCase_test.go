package getComments

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting comments succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			a = []comment.Comment{
				{
					UUID: "article-uuid-1",
					Body: "body-1",
					Author: author.Author{
						UUID: "author-uuid-1",
					},
					ObjectUUID: "object-uuid-1",
					ObjectType: "article",
				},
				{
					UUID: "article-uuid-2",
					Author: author.Author{
						UUID: "author-uuid-2",
					},
				},
				{
					UUID: "article-uuid-3",
					Author: author.Author{
						UUID: "author-uuid-2",
					},
					ApprovedAt: time.Now(),
					CreatedAt:  time.Now(),
				},
			}

			r = Request{
				Page: 0,
			}

			expectedResponse = Response{
				Items: []commentResponse{
					{
						UUID: a[0].UUID,
						Body: a[0].Body,
						Author: authorResponse{
							UUID: a[0].Author.UUID,
						},
						ObjectUUID: "object-uuid-1",
						ObjectType: "article",
						ApprovedAt: a[0].ApprovedAt.Format(time.RFC3339),
						CreatedAt:  a[0].CreatedAt.Format(time.RFC3339),
					},
					{
						UUID: a[1].UUID,
						Author: authorResponse{
							UUID: a[1].Author.UUID,
						},
						ApprovedAt: a[1].ApprovedAt.Format(time.RFC3339),
						CreatedAt:  a[1].CreatedAt.Format(time.RFC3339),
					},
					{
						UUID: a[2].UUID,
						Author: authorResponse{
							UUID: a[2].Author.UUID,
						},
						ApprovedAt: a[2].ApprovedAt.Format(time.RFC3339),
						CreatedAt:  a[2].CreatedAt.Format(time.RFC3339),
					},
				},
				Pagination: pagination{
					CurrentPage: 1,
					TotalPages:  1,
				},
			}

			userUUIDs = []string{a[0].Author.UUID, a[1].Author.UUID, a[2].Author.UUID}

			users = []user.User{
				{UUID: userUUIDs[0]},
				{UUID: userUUIDs[1]},
			}
		)

		commentRepository.On("Count").Once().Return(uint(len(a)), nil)
		commentRepository.On("GetAll", uint(0), uint(10)).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", userUUIDs).Once().Return(users, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting comments fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get articles failed")
		)

		commentRepository.On("Count").Once().Return(uint(0), expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		commentRepository.AssertNotCalled(t, "GetAll")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting comments fails", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get articles failed")
		)

		commentRepository.On("Count").Once().Return(uint(3), nil)
		commentRepository.On("GetAll", uint(0), uint(10)).Return(nil, expectedErr)
		defer commentRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting comment users", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			a = []comment.Comment{
				{
					UUID: "article-uuid-1",
					Body: "body-1",
					Author: author.Author{
						UUID: "author-uuid-1",
					},
					ObjectUUID: "object-uuid-1",
					ObjectType: "article",
				},
				{
					UUID: "article-uuid-2",
					Author: author.Author{
						UUID: "author-uuid-2",
					},
				},
				{
					UUID: "article-uuid-3",
					Author: author.Author{
						UUID: "author-uuid-2",
					},
					ApprovedAt: time.Now(),
					CreatedAt:  time.Now(),
				},
			}

			r = Request{
				Page: 0,
			}

			userUUIDs = []string{a[0].Author.UUID, a[1].Author.UUID, a[2].Author.UUID}

			expectedErr = errors.New("get articles failed")
		)

		commentRepository.On("Count").Once().Return(uint(len(a)), nil)
		commentRepository.On("GetAll", uint(0), uint(10)).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", userUUIDs).Once().Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&commentRepository, &userRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
