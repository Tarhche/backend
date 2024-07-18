package getarticles

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/article"
)

func TestUseCase_GetArticles(t *testing.T) {
	t.Run("returns articles", func(t *testing.T) {
		repository := MockArticlesRepository{}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetArticles(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetAllCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetAllCount)
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

		request := Request{Page: 1}
		response, err := usecase.GetArticles(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetAllCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetAllCount)
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
			GetAllErr: errors.New("article not found"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetArticles(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetAllCount != 0 {
			t.Errorf("unexpected number of calls %d", repository.GetAllCount)
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

	GetAllCount uint
	GetAllErr   error

	GetCountCount uint
	GetCountErr   error
}

func (r *MockArticlesRepository) GetAllPublished(offset uint, limit uint) ([]article.Article, error) {
	r.GetAllCount++

	if r.GetAllErr != nil {
		return nil, r.GetAllErr
	}

	return []article.Article{}, nil
}

func (r *MockArticlesRepository) CountPublished() (uint, error) {
	r.GetCountCount++

	if r.GetAllErr != nil {
		return 0, r.GetAllErr
	}

	return 1, nil
}
