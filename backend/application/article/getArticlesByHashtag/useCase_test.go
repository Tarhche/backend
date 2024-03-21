package getArticlesByHashtag

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/article"
)

func TestUseCase_GetArticles(t *testing.T) {
	t.Run("returns articles by hashtag", func(t *testing.T) {
		repository := MockArticlesRepository{}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1, Hashtag: "test"}
		response, err := usecase.GetArticlesByHashtag(&request)

		if err != nil {
			t.Error("unexpected error")
		}

		if repository.GetByHashtagCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetByHashtagCount)
		}

		if response == nil {
			t.Error("unexpected response")
		}
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		repository := MockArticlesRepository{
			GetByHashtagErr: errors.New("article not found"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1, Hashtag: "test"}
		response, err := usecase.GetArticlesByHashtag(&request)

		if err == nil {
			t.Error("expects an error")
		}

		if repository.GetByHashtagCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetByHashtagCount)
		}

		if response != nil {
			t.Error("unexpected response")
		}
	})
}

type MockArticlesRepository struct {
	article.Repository

	GetByHashtagCount uint
	GetByHashtagErr   error
}

func (r *MockArticlesRepository) GetByHashtag(hashtags []string, offset uint, limit uint) ([]article.Article, error) {
	r.GetByHashtagCount++

	if r.GetByHashtagErr != nil {
		return nil, r.GetByHashtagErr
	}

	return []article.Article{}, nil
}
