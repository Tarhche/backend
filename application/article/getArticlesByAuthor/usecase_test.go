package getArticlesByAuthor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/matcher"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns articles by author username", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, u.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", mock.Anything, authorUUID, "EN").Once().Return(uint(len(a)), nil)
		articleRepository.On("GetPublishedByAuthor", mock.Anything, authorUUID, "EN", uint(0), uint(10)).Once().Return(a, nil)
		elementsRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)
		articleRepository.On("GetPublishedLanguageCodes", mock.Anything, "").Return([]string{}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{}).Return([]language.Language{}, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

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
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, authorUUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", mock.Anything, authorUUID, "EN").Once().Return(uint(len(a)), nil)
		articleRepository.On("GetPublishedByAuthor", mock.Anything, authorUUID, "EN", uint(0), uint(10)).Once().Return(a, nil)
		elementsRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)
		articleRepository.On("GetPublishedLanguageCodes", mock.Anything, "").Return([]string{}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{}).Return([]language.Language{}, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.Items, len(a))
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			request          = Request{Page: 1}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"author": "required_field",
				},
			}
		)

		requestValidator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer requestValidator.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

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
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			request = Request{Username: "ghost", Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Username).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

		articleRepository.AssertNotCalled(t, "CountPublishedByAuthor")
		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)
	})

	t.Run("returns an error on looking up author", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			expectedErr = errors.New("user repo failure")
			request     = Request{Username: "johndoe", Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Username).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

		articleRepository.AssertNotCalled(t, "CountPublishedByAuthor")
		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			authorUUID  = "author-uuid"
			u           = user.User{UUID: authorUUID, Username: "johndoe"}
			expectedErr = errors.New("count failure")
			request     = Request{Username: u.Username, Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, u.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", mock.Anything, authorUUID, "EN").Once().Return(uint(0), expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

		articleRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository   articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			authorUUID  = "author-uuid"
			u           = user.User{UUID: authorUUID, Username: "johndoe"}
			expectedErr = errors.New("get failure")
			request     = Request{Username: u.Username, Page: 1}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, u.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articleRepository.On("CountPublishedByAuthor", mock.Anything, authorUUID, "EN").Once().Return(uint(5), nil)
		articleRepository.On("GetPublishedByAuthor", mock.Anything, authorUUID, "EN", uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articleRepository, &elementsRepository, &userRepository, matcher.New()), &requestValidator).Execute(context.Background(), &request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
