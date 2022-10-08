package article

import (
	"errors"
	"github.com/Tarhche/backend/domain/article"
	"github.com/google/uuid"
)

type InMemoryRepository struct {
	articles []article.Entity
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		articles: []article.Entity{},
	}
}

func (i *InMemoryRepository) Articles() ([]article.Entity, error) {
	return i.articles, nil
}

func (i *InMemoryRepository) CreateArticle(article *article.Entity) error {
	article.ID = uuid.NewString()
	i.articles = append(i.articles, *article)

	return nil
}

func (i *InMemoryRepository) Article(id string) (*article.Entity, error) {
	for j := range i.articles {
		if i.articles[j].ID == id {
			return &i.articles[j], nil
		}
	}

	return nil, errors.New("article not found")
}

func (i *InMemoryRepository) UpdateArticle(article *article.Entity) error {
	for j := range i.articles {
		if i.articles[j].ID == article.ID {
			i.articles[j] = *article

			return nil
		}
	}

	return errors.New("article not found")
}

func (i *InMemoryRepository) DeleteArticle(ID string) error {
	for j := range i.articles {
		if i.articles[j].ID == ID {
			i.articles[j] = i.articles[len(i.articles)-1]
			i.articles = i.articles[:len(i.articles)-1]

			return nil
		}
	}

	return errors.New("article not found")
}
