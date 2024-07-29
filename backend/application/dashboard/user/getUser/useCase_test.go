package getuser

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/article"
)

func TestUseCase_GetArticle(t *testing.T) {
	t.Run("returns an article", func(t *testing.T) {
		repository := MockArticlesRepository{}

		usecase := NewUseCase(&repository)
		response, err := usecase.GetArticle("test-uuid")

		if repository.GetOneCount != 1 {
			t.Error("unexpected number of calls")
		}

		if response == nil {
			t.Error("unexpected response")
		}

		if err != nil {
			t.Error("unexpected error")
		}
	})

	t.Run("returns an error", func(t *testing.T) {
		repository := MockArticlesRepository{
			GetOneErr: errors.New("article not found"),
		}

		usecase := NewUseCase(&repository)
		response, err := usecase.GetArticle("test-uuid")

		if repository.GetOneCount != 1 {
			t.Error("unexpected number of calls")
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

	GetOneCount uint
	GetOneErr   error
}

func (r *MockArticlesRepository) GetOne(UUID string) (article.Article, error) {
	r.GetOneCount++

	if r.GetOneErr != nil {
		return article.Article{}, r.GetOneErr
	}

	return article.Article{}, nil
}
