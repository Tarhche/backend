package repository

import (
	"sync"

	"github.com/khanzadimahdi/testproject.git/domain"
	"github.com/khanzadimahdi/testproject.git/domain/article"
)

type ArticlesRepository struct {
	datastore *sync.Map
}

var _ article.Repository = &ArticlesRepository{}

func NewArticlesRepository(datastore *sync.Map) *ArticlesRepository {
	if datastore == nil {
		panic("datastore should not be nil")
	}

	return &ArticlesRepository{
		datastore: datastore,
	}
}

func (r *ArticlesRepository) GetAll(offset uint, limit uint) ([]article.Article, error) {
	var (
		a []article.Article
		i uint
		j uint
	)

	r.datastore.Range(func(key, value any) bool {
		if i < offset {
			i++
			return true
		}

		if j < limit {
			j++
			a = append(a, value.(article.Article))
		}

		return j < limit
	})

	return a, nil
}

func (r *ArticlesRepository) GetOne(UUID string) (article.Article, error) {
	a, ok := r.datastore.Load(UUID)
	if !ok {
		return article.Article{}, domain.ErrNotExists
	}

	return a.(article.Article), nil
}

func (r *ArticlesRepository) Count() (uint, error) {
	var c uint

	r.datastore.Range(func(_, _ any) bool {
		c++

		return true
	})

	return c, nil
}
