package getArticlesByHashtag

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns articles by hashtag", func(t *testing.T) {
		var (
			repository articles.MockArticlesRepository
			hashtag    = "test-hashtag"
			a          = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}
		)

		repository.On("GetByHashtag", []string{hashtag}, uint(0), uint(10)).Once().Return(a, nil)
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository)
		request := Request{Page: 1, Hashtag: hashtag}
		response, err := usecase.Execute(&request)

		assert.NoError(t, err, "unexpected error")
		assert.NotNil(t, response, "unexpected response")
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		var (
			repository  articles.MockArticlesRepository
			hashtag     = "test-hashtag"
			expectedErr = errors.New("test error")
		)

		repository.On("GetByHashtag", []string{hashtag}, uint(0), uint(10)).Once().Return(nil, expectedErr)
		repository.AssertNotCalled(t, "GetByHashtag")
		defer repository.AssertExpectations(t)

		usecase := NewUseCase(&repository)
		request := Request{Page: 1, Hashtag: hashtag}
		response, err := usecase.Execute(&request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response, "unexpected response")
	})
}
