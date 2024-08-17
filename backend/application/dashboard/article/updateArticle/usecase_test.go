package updatearticle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("updating an articles succeeds", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository

			r = Request{
				UUID:       "test-article-uuid",
				Title:      "test title",
				Excerpt:    "test excerpt",
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				Tags:       []string{"tag1", "tag2"},
			}
			a = article.Article{
				UUID:        r.UUID,
				Cover:       r.Cover,
				Video:       r.Video,
				Title:       r.Title,
				Excerpt:     r.Excerpt,
				Body:        r.Body,
				PublishedAt: r.PublishedAt,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				Tags: r.Tags,
			}

			expectedResponse = Response{}
		)

		articleRepository.On("Save", &a).Once().Return(a.UUID, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository).Execute(r)
		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			r                 = Request{}
			expectedResponse  = Response{
				ValidationErrors: validationErrors{
					"title":   "title is required",
					"excerpt": "excerpt is required",
					"body":    "body is required",
					"author":  "author is required",
				},
			}
		)

		response, err := NewUseCase(&articleRepository).Execute(r)

		articleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("updating an article fails", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			r                 = Request{
				UUID:       "test-article-uuid",
				Title:      "test title",
				Excerpt:    "test excerpt",
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				Tags:       []string{"tag1", "tag2"},
			}
			a = article.Article{
				UUID:        r.UUID,
				Cover:       r.Cover,
				Video:       r.Video,
				Title:       r.Title,
				Excerpt:     r.Excerpt,
				Body:        r.Body,
				PublishedAt: r.PublishedAt,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				Tags: r.Tags,
			}

			expectedErr = errors.New("error happened")
		)

		articleRepository.On("Save", &a).Once().Return("", expectedErr)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository).Execute(r)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
