package createarticle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("creates an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator

			r = Request{
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				Tags:         []string{"tag1", "tag2"},
				LanguageCode: "EN",
			}
			a = article.Article{
				Cover:        r.Cover,
				Video:        r.Video,
				Title:        r.Title,
				Excerpt:      r.Excerpt,
				Body:         r.Body,
				PublishedAt:  r.PublishedAt,
				AuthorUUID:   r.AuthorUUID,
				Tags:         r.Tags,
				LanguageCode: r.LanguageCode,
			}

			u                = "article-uuid"
			expectedResponse = Response{UUID: u}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("Save", &a).Once().Return(u, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"title":         "title is required",
					"excerpt":       "excerpt is required",
					"body":          "body is required",
					"author":        "author is required",
					"language_code": "language is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator).Execute(&r)

		articleRepository.AssertNotCalled(t, "Save")
		languageRepository.AssertNotCalled(t, "Exists")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("language is invalid", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator

			r = Request{
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				LanguageCode: "DE",
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", "DE").Once().Return(false)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator).Execute(&r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "invalid_value", response.ValidationErrors["language_code"])
	})

	t.Run("saving the article fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator

			r = Request{
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				Tags:         []string{"tag1", "tag2"},
				LanguageCode: "EN",
			}
			a = article.Article{
				Cover:        r.Cover,
				Video:        r.Video,
				Title:        r.Title,
				Excerpt:      r.Excerpt,
				Body:         r.Body,
				PublishedAt:  r.PublishedAt,
				AuthorUUID:   r.AuthorUUID,
				Tags:         r.Tags,
				LanguageCode: r.LanguageCode,
			}

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("Save", &a).Once().Return("", expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("creates a translation reusing an existing correlation uuid", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			correlationUUID = "existing-correlation-uuid"

			r = Request{
				Title:           "translation title",
				Excerpt:         "translation excerpt",
				Body:            "translation body",
				AuthorUUID:      "author-uuid",
				LanguageCode:    "FA",
				CorrelationUUID: correlationUUID,
			}
			a = article.Article{
				Cover:           r.Cover,
				Video:           r.Video,
				Title:           r.Title,
				Excerpt:         r.Excerpt,
				Body:            r.Body,
				PublishedAt:     r.PublishedAt,
				AuthorUUID:      r.AuthorUUID,
				Tags:            r.Tags,
				LanguageCode:    r.LanguageCode,
				CorrelationUUID: r.CorrelationUUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("CorrelationExist", correlationUUID).Once().Return(true, nil)
		articleRepository.On("Save", &a).Once().Return("new-uuid", nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &requestValidator).Execute(&r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "new-uuid", response.UUID)
	})

	t.Run("correlation uuid does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			correlationUUID = "ghost-correlation-uuid"

			r = Request{
				Title:           "translation title",
				Excerpt:         "translation excerpt",
				Body:            "translation body",
				AuthorUUID:      "author-uuid",
				LanguageCode:    "FA",
				CorrelationUUID: correlationUUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("CorrelationExist", correlationUUID).Once().Return(false, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &requestValidator).Execute(&r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "invalid_value", response.ValidationErrors["correlation_uuid"])
	})

	t.Run("checking correlation existence fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			correlationUUID = "existing-correlation-uuid"
			expectedErr     = errors.New("error happened")

			r = Request{
				Title:           "translation title",
				Excerpt:         "translation excerpt",
				Body:            "translation body",
				AuthorUUID:      "author-uuid",
				LanguageCode:    "FA",
				CorrelationUUID: correlationUUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("CorrelationExist", correlationUUID).Once().Return(false, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &requestValidator).Execute(&r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
