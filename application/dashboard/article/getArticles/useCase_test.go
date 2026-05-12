package getarticles

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting articles succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			a = []article.Article{
				{
					Title:      "article-title-1",
					UUID:       "article-uuid-1",
					Body:       "body-1",
					Excerpt:    "excerpt-1",
					AuthorUUID: "author-uuid-1",
				},
				{
					UUID:       "article-uuid-2",
					Tags:       []string{"tag-1", "tag-2"},
					AuthorUUID: "author-uuid-2",
				},
				{
					UUID:        "article-uuid-3",
					Tags:        []string{},
					PublishedAt: time.Now(),
					AuthorUUID:  "author-uuid-1",
				},
			}
			u = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1", Username: "author-username-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2", Username: "author-username-2"},
			}

			r = Request{
				Page: 0,
			}

			expectedResponse = Response{
				Items: []articleResponse{
					{
						UUID:        a[0].UUID,
						Title:       a[0].Title,
						PublishedAt: "0001-01-01T00:00:00Z",
						Author: author{
							UUID:     "author-uuid-1",
							Name:     "author-name-1",
							Avatar:   "author-avatar-1",
							Username: "author-username-1",
						},
					},
					{
						UUID:        a[1].UUID,
						PublishedAt: "0001-01-01T00:00:00Z",
						Author: author{
							UUID:     "author-uuid-2",
							Name:     "author-name-2",
							Avatar:   "author-avatar-2",
							Username: "author-username-2",
						},
					},
					{
						UUID:        a[2].UUID,
						PublishedAt: a[2].PublishedAt.Format(time.RFC3339),
						Author: author{
							UUID:     "author-uuid-1",
							Name:     "author-name-1",
							Avatar:   "author-avatar-1",
							Username: "author-username-1",
						},
					},
				},
				Pagination: pagination{
					CurrentPage: 1,
					TotalPages:  1,
				},
			}
		)

		articleRepository.On("Count").Once().Return(uint(len(a)), nil)
		articleRepository.On("GetAll", uint(0), uint(20)).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-2", "author-uuid-1"}).Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting articles fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get articles failed")
		)

		articleRepository.On("Count").Once().Return(uint(0), expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		articleRepository.AssertNotCalled(t, "GetAll")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting articles fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get articles failed")
		)

		articleRepository.On("Count").Once().Return(uint(3), nil)
		articleRepository.On("GetAll", uint(0), uint(20)).Return(nil, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting authors fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			a = []article.Article{
				{UUID: "article-uuid-1", AuthorUUID: "author-uuid-1"},
			}
			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get authors failed")
		)

		articleRepository.On("Count").Once().Return(uint(1), nil)
		articleRepository.On("GetAll", uint(0), uint(20)).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1"}).Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
