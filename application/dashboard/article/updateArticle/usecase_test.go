package updatearticle

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

	t.Run("updating an articles succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				CorrelationUUID: "test-correlation-uuid",
				Title:           "test title",
				Excerpt:         "test excerpt",
				Body:            "test body",
				AuthorUUID:      "test-author-uuid",
				Tags:            []string{"tag1", "tag2"},
				LanguageCode:    "EN",
			}
			existing = article.Article{
				UUID:            "test-article-uuid",
				CorrelationUUID: r.CorrelationUUID,
				Title:           "old title",
				LanguageCode:    "EN",
				ViewCount:       7,
			}
			a = article.Article{
				UUID:            existing.UUID,
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
				ViewCount:       existing.ViewCount,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", mock2.Anything, "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", mock2.Anything, r.CorrelationUUID, r.LanguageCode).Once().Return(existing, nil)
		articleRepository.On("Save", mock2.Anything, &a).Once().Return(a.UUID, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Nil(t, response)
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
					"correlation_uuid": "correlation uuid is required",
					"title":            "title is required",
					"excerpt":          "excerpt is required",
					"body":             "body is required",
					"author":           "author is required",
					"language_code":    "language is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

		articleRepository.AssertNotCalled(t, "Save")
		articleRepository.AssertNotCalled(t, "GetByCorrelationUUIDAndLanguage")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("invalid language fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				CorrelationUUID: "test-correlation-uuid",
				Title:           "test title",
				Excerpt:         "test excerpt",
				Body:            "test body",
				AuthorUUID:      "test-author-uuid",
				LanguageCode:    "DE",
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
		articleRepository.AssertNotCalled(t, "GetByCorrelationUUIDAndLanguage")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "language code is invalid", response.ValidationErrors["language_code"])
	})

	t.Run("updating an article fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				CorrelationUUID: "test-correlation-uuid",
				Title:           "test title",
				Excerpt:         "test excerpt",
				Body:            "test body",
				AuthorUUID:      "test-author-uuid",
				Tags:            []string{"tag1", "tag2"},
				LanguageCode:    "EN",
			}
			existing = article.Article{UUID: "test-article-uuid", CorrelationUUID: r.CorrelationUUID, LanguageCode: "EN"}
			a        = article.Article{
				UUID:            existing.UUID,
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

			expectedErr = errors.New("error happened")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", mock2.Anything, "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", mock2.Anything, r.CorrelationUUID, r.LanguageCode).Once().Return(existing, nil)
		articleRepository.On("Save", mock2.Anything, &a).Once().Return("", expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &languageRepository, &validator, &translator).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
