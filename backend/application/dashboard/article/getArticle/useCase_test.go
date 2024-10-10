package getarticle

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("getting an article succeeds", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository

			articleUUID = "article-uuid"
			a           = article.Article{
				UUID: articleUUID,
			}
			expectedResponse = Response{
				UUID:        articleUUID,
				Tags:        []string{},
				PublishedAt: a.PublishedAt.Format(time.RFC3339),
			}
		)

		articleRepository.On("GetOne", articleUUID).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository).Execute(articleUUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting an article fails", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository

			articleUUID   = "article-uuid"
			expectedError = errors.New("error")
		)

		articleRepository.On("GetOne", articleUUID).Return(article.Article{}, expectedError)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository).Execute(articleUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
