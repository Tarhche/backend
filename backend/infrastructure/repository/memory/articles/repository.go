package repository

import (
	"slices"
	"sync"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
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

		a = append(a, value.(article.Article))
		j++

		return j < limit
	})

	return a, nil
}

func (r *ArticlesRepository) GetByUUIDs(UUIDs []string) ([]article.Article, error) {
	a := make([]article.Article, 0, len(UUIDs))

	r.datastore.Range(func(key, value any) bool {
		if v := value.(article.Article); slices.Contains(UUIDs, v.UUID) {
			a = append(a, value.(article.Article))
		}

		return true
	})

	return a, nil
}

func (r *ArticlesRepository) GetMostViewed(limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) GetByHashtag(hashtags []string, offset uint, limit uint) ([]article.Article, error) {
	return nil, nil
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

func (r *ArticlesRepository) Save(a *article.Article) (string, error) {
	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()
	}

	r.datastore.Store(a.UUID, *a)

	return a.UUID, nil
}

func (r *ArticlesRepository) Delete(UUID string) error {
	r.datastore.Delete(UUID)

	return nil
}

func (r *ArticlesRepository) IncreaseView(uuid string, inc uint) error {

	return nil
}
