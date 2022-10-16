package doubles

import (
	"errors"
	"github.com/Tarhche/backend/domain/article"
	"github.com/google/uuid"
)

type StubArticleRepository struct {
	Entities []article.Entity
}

func (s *StubArticleRepository) Articles() ([]article.Entity, error) {
	return s.Entities, nil
}

func (s *StubArticleRepository) CreateArticle(article *article.Entity) error {
	article.ID = uuid.NewString()
	s.Entities = append(s.Entities, *article)

	return nil
}

func (s *StubArticleRepository) Article(id string) (*article.Entity, error) {
	for j := range s.Entities {
		if s.Entities[j].ID == id {
			return &s.Entities[j], nil
		}
	}

	return nil, errors.New("article not found")
}

func (s *StubArticleRepository) UpdateArticle(article *article.Entity) error {
	for j := range s.Entities {
		if s.Entities[j].ID == article.ID {
			s.Entities[j] = *article
			return nil
		}
	}

	return errors.New("article not found")
}

func (s *StubArticleRepository) DeleteArticle(ID string) error {
	for j := range s.Entities {
		if s.Entities[j].ID == ID {
			s.Entities[j] = s.Entities[len(s.Entities)-1]
			s.Entities = s.Entities[:len(s.Entities)-1]

			return nil
		}
	}

	return errors.New("article not found")
}
