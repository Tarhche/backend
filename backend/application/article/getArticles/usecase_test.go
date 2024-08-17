package getarticles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns articles", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			a                  = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}
		)

		articlesRepository.On("CountPublished").Once().Return(uint(1), nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(10)).Once().Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			expectedErr        = errors.New("test error")
		)

		articlesRepository.On("CountPublished").Once().Return(uint(1), expectedErr)
		defer articlesRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		articlesRepository.AssertNotCalled(t, "GetAllPublished")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			expectedErr        = errors.New("test error")
		)

		articlesRepository.On("CountPublished").Once().Return(uint(1), nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})
}
