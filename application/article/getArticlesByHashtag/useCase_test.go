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

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetByHashtagCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetByHashtagCount)
		}

		if response == nil {
			t.Error("unexpected response")
		}

		if err != nil {
			t.Error("unexpected error")
		}
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		repository := MockArticlesRepository{
			GetCountErr: errors.New("error on counting"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1, Hashtag: "test"}
		response, err := usecase.GetArticlesByHashtag(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetByHashtagCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetByHashtagCount)
		}

		if response == nil {
			t.Error("unexpected response")
		}

		if err != nil {
			t.Error("expects an error")
		}
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		repository := MockArticlesRepository{
			GetByHashtagErr: errors.New("article not found"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1, Hashtag: "test"}
		response, err := usecase.GetArticlesByHashtag(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetByHashtagCount != 0 {
			t.Errorf("unexpected number of calls %d", repository.GetByHashtagCount)
		}

		if response != nil {
			t.Error("unexpected response")
		}

		if err == nil {
			t.Error("expects an error")
		}
	})
}

type MockArticlesRepository struct {
	article.Repository

	GetByHashtagCount uint
	GetByHashtagErr   error

	GetCountCount uint
	GetCountErr   error
}

func (r *MockArticlesRepository) GetByHashtag(hashtags []string, offset uint, limit uint) ([]article.Article, error) {
	r.GetByHashtagCount++

	if r.GetByHashtagErr != nil {
		return nil, r.GetByHashtagErr
	}

	return []article.Article{}, nil
}
