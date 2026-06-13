package getarticles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting articles succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			correlationUUIDs = []string{"correlation-1", "correlation-2"}

			a = []article.Article{
				{UUID: "a1", CorrelationUUID: "correlation-1", LanguageCode: "EN", Cover: "cover-1", Title: "title-1-en", AuthorUUID: "author-1"},
				{UUID: "a2", CorrelationUUID: "correlation-1", LanguageCode: "FA", Title: "title-1-fa", AuthorUUID: "author-1"},
				{UUID: "a3", CorrelationUUID: "correlation-2", LanguageCode: "EN", Title: "title-2-en", AuthorUUID: "author-2"},
			}
			u = []user.User{
				{UUID: "author-1", Name: "Author One", Avatar: "a1.png", Username: "author_one"},
				{UUID: "author-2", Name: "Author Two", Avatar: "a2.png", Username: "author_two"},
			}
			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "Persian"},
			}

			r = Request{Page: 0}

			expectedResponse = Response{
				Items: []articleResponse{
					{
						CorrelationUUID: "correlation-1",
						CorrolatedItems: []corrolatedArticleResponse{
							{
								Cover:       "cover-1",
								Title:       "title-1-en",
								PublishedAt: "0001-01-01T00:00:00Z",
								Author:      author{UUID: "author-1", Name: "Author One", Avatar: "a1.png", Username: "author_one"},
								Language:    languageResponse{Code: "EN", Name: "English"},
							},
							{
								Title:       "title-1-fa",
								PublishedAt: "0001-01-01T00:00:00Z",
								Author:      author{UUID: "author-1", Name: "Author One", Avatar: "a1.png", Username: "author_one"},
								Language:    languageResponse{Code: "FA", Name: "Persian"},
							},
						},
					},
					{
						CorrelationUUID: "correlation-2",
						CorrolatedItems: []corrolatedArticleResponse{
							{
								Title:       "title-2-en",
								PublishedAt: "0001-01-01T00:00:00Z",
								Author:      author{UUID: "author-2", Name: "Author Two", Avatar: "a2.png", Username: "author_two"},
								Language:    languageResponse{Code: "EN", Name: "English"},
							},
						},
					},
				},
				Pagination: pagination{CurrentPage: 1, TotalPages: 1},
			}
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(2), nil)
		articleRepository.On("GetCorrelationUUIDs", uint(0), uint(20)).Once().Return(correlationUUIDs, nil)
		articleRepository.On("GetByCorrelationUUIDs", correlationUUIDs, "").Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-1", "author-1", "author-2"}).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		languageRepository.On("GetByCodes", []string{"EN", "FA", "EN"}).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			r = Request{Page: 0}

			expectedResponse = Response{
				Items:      []articleResponse{},
				Pagination: pagination{CurrentPage: 1, TotalPages: 0},
			}
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(0), nil)
		articleRepository.On("GetCorrelationUUIDs", uint(0), uint(20)).Once().Return([]string{}, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		articleRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetByUUIDs")
		languageRepository.AssertNotCalled(t, "GetByCodes")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting correlations fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			r           = Request{Page: 0}
			expectedErr = errors.New("count failed")
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(0), expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		articleRepository.AssertNotCalled(t, "GetCorrelationUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting correlation uuids fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			r           = Request{Page: 0}
			expectedErr = errors.New("get correlations failed")
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(2), nil)
		articleRepository.On("GetCorrelationUUIDs", uint(0), uint(20)).Once().Return(nil, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		articleRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting articles fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			correlationUUIDs = []string{"correlation-1"}
			r                = Request{Page: 0}
			expectedErr      = errors.New("get articles failed")
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(1), nil)
		articleRepository.On("GetCorrelationUUIDs", uint(0), uint(20)).Once().Return(correlationUUIDs, nil)
		articleRepository.On("GetByCorrelationUUIDs", correlationUUIDs, "").Once().Return(nil, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting authors fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			correlationUUIDs = []string{"correlation-1"}
			a                = []article.Article{{UUID: "a1", CorrelationUUID: "correlation-1", LanguageCode: "EN", AuthorUUID: "author-1"}}
			r                = Request{Page: 0}
			expectedErr      = errors.New("get authors failed")
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(1), nil)
		articleRepository.On("GetCorrelationUUIDs", uint(0), uint(20)).Once().Return(correlationUUIDs, nil)
		articleRepository.On("GetByCorrelationUUIDs", correlationUUIDs, "").Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-1"}).Once().Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		languageRepository.AssertNotCalled(t, "GetByCodes")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			correlationUUIDs = []string{"correlation-1"}
			a                = []article.Article{{UUID: "a1", CorrelationUUID: "correlation-1", LanguageCode: "EN", AuthorUUID: "author-1"}}
			r                = Request{Page: 0}
			expectedErr      = errors.New("get languages failed")
		)

		articleRepository.On("CountByCorrelation").Once().Return(uint(1), nil)
		articleRepository.On("GetCorrelationUUIDs", uint(0), uint(20)).Once().Return(correlationUUIDs, nil)
		articleRepository.On("GetByCorrelationUUIDs", correlationUUIDs, "").Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-1"}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		languageRepository.On("GetByCodes", []string{"EN"}).Once().Return(nil, expectedErr)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
