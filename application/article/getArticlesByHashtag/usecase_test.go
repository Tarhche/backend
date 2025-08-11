package getArticlesByHashtag

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns articles by hashtag", func(t *testing.T) {
		t.Parallel()

		var (
			repository articles.MockArticlesRepository
			validator  validator.MockValidator

			hashtag = "test-hashtag"
			a       = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}
			request = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}).Once().Return(uint(len(a)), nil)
		defer repository.AssertExpectations(t)

		repository.On("GetPublishedByHashtags", []string{hashtag}, uint(0), uint(10)).Once().Return(a, nil)
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository, &validator)
		response, err := usecase.Execute(&request)

		assert.NoError(t, err, "unexpected error")
		assert.NotNil(t, response, "unexpected response")
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			repository articles.MockArticlesRepository
			validator  validator.MockValidator

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

		usecase := NewUseCase(&repository, &validator)
		response, err := usecase.Execute(&request)

		repository.AssertNotCalled(t, "CountPublishedByHashtags")
		repository.AssertNotCalled(t, "GetPublishedByHashtags")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		t.Parallel()

		var (
			repository articles.MockArticlesRepository
			validator  validator.MockValidator

			hashtag     = "test-hashtag"
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}).Once().Return(uint(0), expectedErr)
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository, &validator)
		response, err := usecase.Execute(&request)

		repository.AssertNotCalled(t, "GetPublishedByHashtags")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response, "unexpected response")
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		t.Parallel()

		var (
			repository articles.MockArticlesRepository
			validator  validator.MockValidator

			hashtag     = "test-hashtag"
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		repository.On("CountPublishedByHashtags", []string{hashtag}).Once().Return(uint(5), nil)
		defer repository.AssertExpectations(t)

		repository.On("GetPublishedByHashtags", []string{hashtag}, uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository, &validator)
		response, err := usecase.Execute(&request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response, "unexpected response")
	})
}
