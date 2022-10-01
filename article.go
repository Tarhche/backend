package main

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

type Article struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

type ArticleRepository interface {
	Articles() ([]Article, error)
	CreateArticle(*Article) error
	Article(id string) (*Article, error)
	UpdateArticle(*Article) error
	DeleteArticle(string) error
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		articles: []Article{},
	}
}

type InMemoryRepository struct {
	articles []Article
}

func (i *InMemoryRepository) Articles() ([]Article, error) {
	return i.articles, nil
}

func (i *InMemoryRepository) CreateArticle(article *Article) error {
	article.ID = uuid.NewString()
	i.articles = append(i.articles, *article)

	return nil
}

func (i *InMemoryRepository) Article(id string) (*Article, error) {
	for j := range i.articles {
		if i.articles[j].ID == id {
			return &i.articles[j], nil
		}
	}

	return nil, errors.New("article not found")
}

func (i *InMemoryRepository) UpdateArticle(article *Article) error {
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

type ArticleServer struct {
	repository ArticleRepository
}

func (a *ArticleServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.createArticle(rw, r)
	} else if r.Method == http.MethodGet {
		id := strings.TrimPrefix(r.URL.Path, "/articles")

		if len(id) == 0 {
			a.articles(rw, r)
		} else {
			id := strings.TrimPrefix(id, "/")
			a.article(rw, id)
		}
	} else if r.Method == http.MethodPut {
		a.update(rw, r)
	} else if r.Method == http.MethodDelete {
		id := strings.TrimPrefix(r.URL.Path, "/articles")
		if len(id) == 0 {
			rw.WriteHeader(http.StatusNotFound)
		} else {
			id := strings.TrimPrefix(id, "/")
			a.delete(rw, id)
		}

	} else {
		rw.WriteHeader(http.StatusNotFound)
	}
}

func (a *ArticleServer) articles(rw http.ResponseWriter, r *http.Request) {
	articles, _ := a.repository.Articles()
	_ = json.NewEncoder(rw).Encode(articles)
}

func (a *ArticleServer) createArticle(rw http.ResponseWriter, r *http.Request) {
	var article Article

	_ = json.NewDecoder(r.Body).Decode(&article)
	_ = a.repository.CreateArticle(&article)

	rw.WriteHeader(http.StatusCreated)
}

func (a *ArticleServer) article(rw http.ResponseWriter, id string) {
	article, err := a.repository.Article(id)
	if err != nil {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(rw).Encode(article)
}

func (a *ArticleServer) update(rw http.ResponseWriter, r *http.Request) {
	article := Article{}

	_ = json.NewDecoder(r.Body).Decode(&article)
	_ = a.repository.UpdateArticle(&article)

	rw.WriteHeader(http.StatusNoContent)
}

func (a *ArticleServer) delete(rw http.ResponseWriter, id string) {
	_ = a.repository.DeleteArticle(id)
	rw.WriteHeader(http.StatusNoContent)
}
