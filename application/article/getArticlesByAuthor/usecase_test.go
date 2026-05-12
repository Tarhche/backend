package getArticlesByAuthor

import (
	"errors"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns articles by author username", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			authorUUID = "author-uuid"
			createdAt  = time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
			u          = user.User{
				UUID:      authorUUID,
				Name:      "author-name",
				Avatar:    "author-avatar",
				Username:  "author-username",
				CreatedAt: createdAt,
			}
			a = []article.Article{
				{UUID: "article-uuid-1", AuthorUUID: authorUUID},
				{UUID: "article-uuid-2", AuthorUUID: authorUUID},
			}

			request = Request{Username: u.Username, Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", authorUUID).Once().Return(uint(len(a)), nil)
		articleRepository.On("GetPublishedByAuthor", authorUUID, uint(0), uint(10)).Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, u.UUID, response.Author.UUID)
		assert.Equal(t, u.Name, response.Author.Name)
		assert.Equal(t, u.Avatar, response.Author.Avatar)
		assert.Equal(t, u.Username, response.Author.Username)
		assert.Equal(t, createdAt.Format(time.RFC3339), response.Author.CreatedAt)
		assert.Len(t, response.Items, len(a))
	})

	t.Run("returns articles by author uuid", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			authorUUID = "author-uuid"
			u          = user.User{
				UUID:     authorUUID,
				Name:     "author-name",
				Username: "author-username",
			}
			a = []article.Article{
				{UUID: "article-uuid-1", AuthorUUID: authorUUID},
			}

			request = Request{AuthorUUID: authorUUID, Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", authorUUID).Once().Return(uint(len(a)), nil)
		articleRepository.On("GetPublishedByAuthor", authorUUID, uint(0), uint(10)).Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.Items, len(a))
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			request          = Request{Page: 1}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"author": "required_field",
				},
			}
		)

		requestValidator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer requestValidator.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		userRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		articleRepository.AssertNotCalled(t, "CountPublishedByAuthor")
		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("author not found returns not-exists error", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			request = Request{Username: "ghost", Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Username).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		articleRepository.AssertNotCalled(t, "CountPublishedByAuthor")
		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)
	})

	t.Run("returns an error on looking up author", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			expectedErr = errors.New("user repo failure")
			request     = Request{Username: "johndoe", Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", request.Username).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		articleRepository.AssertNotCalled(t, "CountPublishedByAuthor")
		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			authorUUID  = "author-uuid"
			u           = user.User{UUID: authorUUID, Username: "johndoe"}
			expectedErr = errors.New("count failure")
			request     = Request{Username: u.Username, Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", authorUUID).Once().Return(uint(0), expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository
			requestValidator  validator.MockValidator

			authorUUID  = "author-uuid"
			u           = user.User{UUID: authorUUID, Username: "johndoe"}
			expectedErr = errors.New("get failure")
			request     = Request{Username: u.Username, Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", authorUUID).Once().Return(uint(5), nil)
		articleRepository.On("GetPublishedByAuthor", authorUUID, uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &requestValidator).Execute(&request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
