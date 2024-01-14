package repository

import (
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
)

type AuthorsRepository struct {
	datastore *sync.Map
}

var _ author.Repository = &AuthorsRepository{}

func NewAuthorsRepository(datastore *sync.Map) *AuthorsRepository {
	if datastore == nil {
		panic("datastore should not be nil")
	}

	return &AuthorsRepository{
		datastore: datastore,
	}
}

func (r *AuthorsRepository) GetOne(UUID string) (author.Author, error) {
	a, ok := r.datastore.Load(UUID)
	if !ok {
		return author.Author{}, domain.ErrNotExists
	}

	return a.(author.Author), nil
}
