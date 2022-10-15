package article

import (
	"errors"
	"github.com/Tarhche/backend/domain/article"
	"github.com/google/uuid"
	"sync"
)

type InMemoryRepository struct {
	articles []article.Entity
	rwLock   sync.RWMutex
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		articles: []article.Entity{},
	}
}

func (i *InMemoryRepository) Articles() ([]article.Entity, error) {
	i.rwLock.RLock()
	defer i.rwLock.RUnlock()

	return i.articles, nil
}

func (i *InMemoryRepository) CreateArticle(article *article.Entity) error {
	i.rwLock.Lock()
	defer i.rwLock.Unlock()

	article.ID = uuid.NewString()
	i.articles = append(i.articles, *article)

	return nil
}

func (i *InMemoryRepository) Article(id string) (*article.Entity, error) {
	i.rwLock.RLock()
	defer i.rwLock.RUnlock()

	for j := range i.articles {
		if i.articles[j].ID == id {
			return &i.articles[j], nil
		}
	}

	return nil, errors.New("article not found")
}

func (i *InMemoryRepository) UpdateArticle(article *article.Entity) error {
	i.rwLock.Lock()
	defer i.rwLock.Unlock()

	for j := range i.articles {
		if i.articles[j].ID == article.ID {
			i.articles[j] = *article

			return nil
		}
	}

	return errors.New("article not found")
}

func (i *InMemoryRepository) DeleteArticle(ID string) error {
	i.rwLock.Lock()
	defer i.rwLock.Unlock()

	for j := range i.articles {
		if i.articles[j].ID == ID {
			i.articles[j] = i.articles[len(i.articles)-1]
			i.articles = i.articles[:len(i.articles)-1]

			return nil
		}
	}

	return errors.New("article not found")
}
