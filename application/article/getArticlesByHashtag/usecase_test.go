package getArticlesByHashtag

import (
	"errors"
	"testing"

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
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns articles by hashtag", func(t *testing.T) {
		t.Parallel()

		var (
			repository          articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			hashtag = "test-hashtag"
			a       = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
				{UUID: "test-article-3", AuthorUUID: "author-uuid-1"},
			}
			u = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2"},
			}
			request = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}, "EN").Once().Return(uint(len(a)), nil)
		repository.On("GetPublishedByHashtags", []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(a, nil)
		elementsRepository.On("Count").Once().Return(uint(0), nil)
		defer repository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-2", "author-uuid-1"}).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		repository.On("GetPublishedLanguageCodes", "").Return([]string{}, nil)
		languagesRepository.On("GetByCodes", []string{}).Return([]language.Language{}, nil)

		usecase := NewUseCase(&repository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&repository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(&request)

		assert.NoError(t, err, "unexpected error")
		assert.NotNil(t, response, "unexpected response")
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			repository          articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			hashtag = "test-hashtag"
			request = Request{Page: 1, Hashtag: hashtag}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"hashtag": "this field is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		usecase := NewUseCase(&repository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&repository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(&request)

		repository.AssertNotCalled(t, "CountPublishedByHashtags")
		repository.AssertNotCalled(t, "GetPublishedByHashtags")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		t.Parallel()

		var (
			repository          articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			hashtag     = "test-hashtag"
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}, "EN").Once().Return(uint(0), expectedErr)
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&repository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(&request)

		repository.AssertNotCalled(t, "GetPublishedByHashtags")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response, "unexpected response")
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		t.Parallel()

		var (
			repository          articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			hashtag     = "test-hashtag"
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}, "EN").Once().Return(uint(5), nil)
		repository.On("GetPublishedByHashtags", []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&repository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(&request)

		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response, "unexpected response")
	})

	t.Run("returns an error on getting authors", func(t *testing.T) {
		t.Parallel()

		var (
			repository          articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			hashtag     = "test-hashtag"
			expectedErr = errors.New("test error")
			a           = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
			}
			request = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}, "EN").Once().Return(uint(1), nil)
		repository.On("GetPublishedByHashtags", []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(a, nil)
		elementsRepository.On("Count").Once().Return(uint(0), nil)
		defer repository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1"}).Once().Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		usecase := NewUseCase(&repository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&repository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(&request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response, "unexpected response")
	})
}
