package getarticle

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
)

func TestUseCase_GetArticle(t *testing.T) {
	t.Run("returns an article", func(t *testing.T) {
		articlesRepository := MockArticlesRepository{}
		elementsRepository := MockElementsRepository{}

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.GetArticle("test-uuid")

		if articlesRepository.GetOneCount != 1 {
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
		articlesRepository := MockArticlesRepository{
			GetOneErr: errors.New("article not found"),
		}

		elementsRepository := MockElementsRepository{}

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.GetArticle("test-uuid")

		if articlesRepository.GetOneCount != 1 {
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

func (r *MockArticlesRepository) IncreaseView(UUID string, inc uint) error {
	return nil
}

func (r *MockArticlesRepository) GetByUUIDs(UUIDS []string) ([]article.Article, error) {
	return nil, nil
}

type MockElementsRepository struct {
	element.Repository

	GetByVenuesCount uint
	GetByVenuesErr   error
}

func (r *MockElementsRepository) GetByVenues(venues []string) ([]element.Element, error) {
	r.GetByVenuesCount++

	if r.GetByVenuesErr != nil {
		return nil, r.GetByVenuesErr
	}

	return []element.Element{}, nil
}
