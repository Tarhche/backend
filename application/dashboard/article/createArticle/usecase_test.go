package createarticle

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeFor[func(*translatorContract.Params)]().Name()

	t.Run("creates an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				Tags:         []string{"tag1", "tag2"},
				LanguageCode: "EN",
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", mock2.Anything, "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		// no correlation uuid is provided, so the use case generates one and the
		// repository stores the article with it (it is not derived from the uuid).
		articleRepository.On("Save", mock2.Anything, mock2.MatchedBy(func(a *article.Article) bool {
			return a.Title == r.Title &&
				a.Excerpt == r.Excerpt &&
				a.Body == r.Body &&
				a.AuthorUUID == r.AuthorUUID &&
				a.LanguageCode == r.LanguageCode &&
				len(a.CorrelationUUID) > 0
		})).Once().Return("article-uuid", nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.NotEmpty(t, response.CorrelationUUID)
		assert.Equal(t, r.CorrelationUUID, response.CorrelationUUID)
		assert.Equal(t, r.LanguageCode, response.LanguageCode)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

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

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

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
			translator         translator.TranslatorMock

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

		translator.On(
			"Translate",
			"invalid_value",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return("language code is invalid")
		defer translator.AssertExpectations(t)

		languageRepository.On("Exists", mock2.Anything, "DE").Once().Return(false)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "language code is invalid", response.ValidationErrors["language_code"])
	})

	t.Run("saving the article fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				Tags:         []string{"tag1", "tag2"},
				LanguageCode: "EN",
			}

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", mock2.Anything, "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("Save", mock2.Anything, mock2.AnythingOfType("*article.Article")).Once().Return("", expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("creates a translation reusing an existing correlation uuid", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator
			translator         translator.TranslatorMock

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

		languageRepository.On("Exists", mock2.Anything, "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("CorrelationExist", mock2.Anything, correlationUUID).Once().Return(true, nil)
		articleRepository.On("Save", mock2.Anything, &a).Once().Return("new-uuid", nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &requestValidator, &translator).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, correlationUUID, response.CorrelationUUID)
		assert.Equal(t, r.LanguageCode, response.LanguageCode)
	})

	t.Run("correlation uuid does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator
			translator         translator.TranslatorMock

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

		translator.On(
			"Translate",
			"invalid_value",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return("correlation uuid is invalid")
		defer translator.AssertExpectations(t)

		languageRepository.On("Exists", mock2.Anything, "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("CorrelationExist", mock2.Anything, correlationUUID).Once().Return(false, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &requestValidator, &translator).Execute(context.Background(), &r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "correlation uuid is invalid", response.ValidationErrors["correlation_uuid"])
	})

	t.Run("checking correlation existence fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator
			translator         translator.TranslatorMock

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

		languageRepository.On("Exists", mock2.Anything, "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("CorrelationExist", mock2.Anything, correlationUUID).Once().Return(false, expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &requestValidator, &translator).Execute(context.Background(), &r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
